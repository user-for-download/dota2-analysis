package httpclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/httpdo"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
)

type ProxiedConfig struct {
	Pool            proxy.Pool
	Hold            time.Duration
	Timeout         time.Duration
	Fallback        *http.Client
	AllowDirect     bool
	MaxRetries      int
	Backoff         time.Duration
	TransportCache  int
	Logger          *slog.Logger
}

type Proxied struct {
	doer *httpdo.Doer
	log  *slog.Logger
}

func NewProxied(cfg ProxiedConfig) (*Proxied, error) {
	doer, err := httpdo.New(httpdo.Config{
		Pool:        cfg.Pool,
		Hold:        cfg.Hold,
		Timeout:     cfg.Timeout,
		MaxRetries:  cfg.MaxRetries,
		Backoff:     cfg.Backoff,
		AllowDirect: cfg.AllowDirect,
		Logger:      cfg.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("httpclient proxied: %w", err)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Proxied{doer: doer, log: log}, nil
}

var _ HTTPClient = (*Proxied)(nil)

func (p *Proxied) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	p.log.Debug("enrich.httpclient: starting request", "url", rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		p.log.Error("enrich.httpclient: failed to build request", "url", rawURL, "err", err)
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/enricher")

	start := time.Now()
	resp, err := p.doer.Do(ctx, req)
	duration := time.Since(start)

	if err != nil {
		p.log.Debug("enrich.httpclient: request failed", "url", rawURL, "err", err, "duration_ms", duration.Milliseconds())
		return nil, err
	}

	p.log.Debug("enrich.httpclient: request successful", "url", rawURL, "status", resp.StatusCode, "duration_ms", duration.Milliseconds())
	return resp, nil
}

func (p *Proxied) Close() error {
	return p.doer.Close()
}