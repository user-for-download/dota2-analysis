package proxyclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/user-for-download/go-dota2/internal/client"
	"github.com/user-for-download/go-dota2/internal/proxy"
)

type Config struct {
	Pool        proxy.Pool
	UpstreamURL string
	Timeout     time.Duration
	MaxRetries  int
	Backoff     time.Duration
	AllowDirect bool
	Logger      *slog.Logger
}

type MatchClient struct {
	cfg     Config
	log     *slog.Logger
	http    *http.Client
	baseURL string
}

func NewMatchClient(cfg Config) (*MatchClient, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("proxyclient: pool is required")
	}
	if cfg.UpstreamURL == "" {
		return nil, fmt.Errorf("proxyclient: upstream URL is required")
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 250 * time.Millisecond
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	rt := NewRoundTripper(cfg.Pool, 30*time.Second, cfg.AllowDirect, cfg.Timeout)

	return &MatchClient{
		cfg:     cfg,
		log:     log.With("component", "proxyclient.match"),
		http:    &http.Client{Transport: rt, Timeout: cfg.Timeout},
		baseURL: cfg.UpstreamURL,
	}, nil
}

var _ client.MatchClient = (*MatchClient)(nil)

func (c *MatchClient) GetMatch(ctx context.Context, matchID int64) ([]byte, error) {
	targetURL := c.baseURL + "/" + strconv.FormatInt(matchID, 10)

	var lastErr error
	for attempt := 0; attempt < c.cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		body, err := c.doRequest(ctx, targetURL)
		if err == nil {
			return body, nil
		}
		lastErr = err

		if isPermanentHTTP(err) {
			return nil, err
		}

		c.log.Debug("request failed, retrying", "match_id", matchID, "attempt", attempt+1, "err", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.cfg.Backoff):
		}
	}

	return nil, fmt.Errorf("exhausted %d attempts: %w", c.cfg.MaxRetries, lastErr)
}

func (c *MatchClient) doRequest(ctx context.Context, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/match-client")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, &httpStatusError{code: resp.StatusCode}
	}

	return io.ReadAll(resp.Body)
}

type httpStatusError struct {
	code int
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("http %d", e.code) }

func (e *httpStatusError) IsPermanent() bool {
	switch e.code {
	case 404, 400, 401, 403, 410:
		return true
	}
	return false
}

func isPermanentHTTP(err error) bool {
	var se *httpStatusError
	return errors.As(err, &se) && se.IsPermanent()
}