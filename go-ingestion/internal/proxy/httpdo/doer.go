package httpdo

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/transport"
)

type Config struct {
	Pool        proxy.Pool
	Hold        time.Duration
	Timeout     time.Duration
	MaxRetries  int
	Backoff     time.Duration
	AllowDirect bool
	Logger      *slog.Logger
}

type Doer struct {
	cfg Config
	log *slog.Logger

	mu          sync.Mutex
	transports  map[string]*httpClientEntry
	accessOrder []string
	stopCh      chan struct{}
}

type httpClientEntry struct {
	client   *http.Client
	close    func()
	lastUsed time.Time
}

func New(cfg Config) (*Doer, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("httpdo: pool required")
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 250 * time.Millisecond
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	d := &Doer{
		cfg:        cfg,
		log:        log.With("component", "httpdo"),
		transports: make(map[string]*httpClientEntry),
		stopCh:     make(chan struct{}),
	}
	go d.cleanupLoop()
	return d, nil
}

// isProxyFault reports whether err is a transport-level failure that is
// attributable to the proxy rather than the target server.  These errors
// warrant switching to a different proxy on the next attempt; retrying the
// same proxy would just fail again for the same reason.
//
// Covered cases:
//   - x509 / TLS errors  — proxy has a wrong system clock or bad cert store,
//     so TLS to the target cannot be verified.
//   - tls.RecordHeaderError — proxy is speaking plain HTTP on a TLS port.
func isProxyFault(err error) bool {
	if err == nil {
		return false
	}
	var x509Err x509.CertificateInvalidError
	if errors.As(err, &x509Err) {
		return true
	}
	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return true
	}
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}
	var tlsRec tls.RecordHeaderError
	if errors.As(err, &tlsRec) {
		return true
	}
	return false
}

func (d *Doer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Per-attempt proxy acquisition: each iteration of the retry loop gets its
	// own fresh proxy lease so that a transport-level failure (TLS, EOF, connect
	// refused) on one proxy does not cause all remaining retries to hit the same
	// broken proxy.
	var lastErr error
	for attempt := 1; attempt <= d.cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := d.doWithLease(ctx, req, attempt)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Permanent HTTP status — no point retrying at all.
		var perr *permanentHTTPError
		if errors.As(err, &perr) && perr.IsPermanent() {
			return nil, err
		}
		// Rate-limited — surface immediately so the caller can handle it.
		if errors.Is(err, proxy.ErrRateLimited) {
			return nil, err
		}
		// Context cancelled/deadline — stop now.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if isProxyFault(err) {
			d.log.Debug("proxy-fault error, switching proxy",
				"attempt", attempt,
				"max_retries", d.cfg.MaxRetries,
				"err", err,
			)
			continue
		}

		d.log.Debug("request failed, retrying with new proxy",
			"attempt", attempt,
			"max_retries", d.cfg.MaxRetries,
			"err", err,
		)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(d.cfg.Backoff):
		}
	}

	if d.cfg.AllowDirect {
		d.log.Warn("all proxy attempts failed, falling back to direct",
			"url", req.URL.String(),
			"err", lastErr,
		)
		return d.doDirect(ctx, req)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("exhausted %d proxy attempts", d.cfg.MaxRetries)
	}
	return nil, lastErr
}

func (d *Doer) doWithLease(ctx context.Context, req *http.Request, attempt int) (*http.Response, error) {
	// Rewind request body on retry — the first attempt consumes req.Body,
	// subsequent attempts need a fresh copy. See https://pkg.go.dev/net/http#Request.GetBody
	if attempt > 1 && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("rewind request body: %w", err)
		}
		req.Body = body
	}

	lease, err := d.cfg.Pool.Acquire(ctx, d.cfg.Hold)
	if err != nil {
		if errors.Is(err, proxy.ErrNoProxy) {
			if !d.cfg.AllowDirect {
				return nil, fmt.Errorf("no proxy available: %w", err)
			}
			return d.doDirect(ctx, req)
		}
		return nil, fmt.Errorf("acquire proxy: %w", err)
	}
	defer lease.Release(context.WithoutCancel(ctx))

	proxyURL := lease.URL
	client, err := d.getClient(proxyURL)
	if err != nil {
		lease.MarkFailure(context.WithoutCancel(ctx), err)
		return nil, fmt.Errorf("proxy transport: %w", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			if resp.StatusCode == http.StatusTooManyRequests {
				err = fmt.Errorf("%w: HTTP %d", proxy.ErrRateLimited, resp.StatusCode)
				lease.MarkFailure(context.WithoutCancel(ctx), err)
				return nil, err
			}
			perr := &permanentHTTPError{StatusCode: resp.StatusCode}
			lease.MarkFailure(context.WithoutCancel(ctx), perr)
			return nil, perr
		}

		// Eager-read the body inside the lease boundary to catch slow-proxy timeouts
		// and guarantee the proxy is not overloaded by premature release.
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lease.MarkFailure(context.WithoutCancel(ctx), fmt.Errorf("read body: %w", err))
			return nil, err
		}

		// Reconstruct the response body with an in-memory buffer so downstream consumers
		// can read it instantly without any further network calls.
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		lease.MarkSuccess(context.WithoutCancel(ctx))
		return resp, nil
	}

	lease.MarkFailure(context.WithoutCancel(ctx), err)
	return nil, err
}

func (d *Doer) doDirect(ctx context.Context, req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	directEntry := d.transports["__direct__"]
	if directEntry == nil {
		baseTr := http.DefaultTransport
		tr, ok := baseTr.(*http.Transport)
		if !ok {
			d.mu.Unlock()
			return nil, fmt.Errorf("direct: expected *http.Transport, got %T", baseTr)
		}
		cloned := tr.Clone()
		otelTr := otelhttp.NewTransport(cloned)
		client := &http.Client{Transport: otelTr, Timeout: d.cfg.Timeout}
		directEntry = &httpClientEntry{
			client:   client,
			close:    func() { cloned.CloseIdleConnections() },
			lastUsed: time.Now(),
		}
		d.transports["__direct__"] = directEntry
	}
	directEntry.lastUsed = time.Now()
	d.mu.Unlock()
	return directEntry.client.Do(req)
}

func (d *Doer) getClient(proxyURL string) (*http.Client, error) {
	pu, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.transports[proxyURL]; ok {
		entry.lastUsed = time.Now()
		d.touchAccessOrder(proxyURL)
		return entry.client, nil
	}

	tr, err := transport.For(pu, d.cfg.Timeout)
	if err != nil {
		return nil, err
	}

	otelTr := otelhttp.NewTransport(tr)
	client := &http.Client{Transport: otelTr, Timeout: d.cfg.Timeout}

	const maxCacheEntries = 100
	if len(d.transports) >= maxCacheEntries && len(d.accessOrder) > 0 {
		evict := d.accessOrder[0]
		if entry, ok := d.transports[evict]; ok {
			if entry.close != nil {
				entry.close()
			}
			delete(d.transports, evict)
		}
		d.accessOrder = d.accessOrder[1:]
	}

	d.transports[proxyURL] = &httpClientEntry{
		client:   client,
		close:    func() { tr.CloseIdleConnections() },
		lastUsed: time.Now(),
	}
	d.accessOrder = append(d.accessOrder, proxyURL)
	return client, nil
}

func (d *Doer) touchAccessOrder(key string) {
	for i, k := range d.accessOrder {
		if k == key {
			d.accessOrder = append(d.accessOrder[:i], d.accessOrder[i+1:]...)
			break
		}
	}
	d.accessOrder = append(d.accessOrder, key)
}

func (d *Doer) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.evictIdleEntries()
		}
	}
}

func (d *Doer) evictIdleEntries() {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for url, entry := range d.transports {
		if now.Sub(entry.lastUsed) > 10*time.Minute {
			if entry.close != nil {
				entry.close()
			}
			delete(d.transports, url)
		}
	}
	newOrder := make([]string, 0, len(d.accessOrder))
	for _, url := range d.accessOrder {
		if _, ok := d.transports[url]; ok {
			newOrder = append(newOrder, url)
		}
	}
	d.accessOrder = newOrder
}

func (d *Doer) Close() error {
	close(d.stopCh)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, entry := range d.transports {
		if entry.close != nil {
			entry.close()
		}
	}
	d.transports = make(map[string]*httpClientEntry)
	d.accessOrder = nil
	return nil
}

type permanentHTTPError struct {
	StatusCode int
}

func (e *permanentHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func (e *permanentHTTPError) IsPermanent() bool {
	return e.StatusCode == http.StatusBadRequest || e.StatusCode == http.StatusUnauthorized ||
		e.StatusCode == http.StatusForbidden || e.StatusCode == http.StatusNotFound ||
		e.StatusCode == http.StatusGone
}

// PermanentHTTPError is exposed for callers to classify failures.
type PermanentHTTPError = permanentHTTPError

func (e *permanentHTTPError) Code() int { return e.StatusCode }