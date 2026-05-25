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

type Direct struct {
	Client *http.Client
}

func (d *Direct) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	return d.Client.Do(req)
}

var _ HTTPClient = (*Direct)(nil)

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
	return &Proxied{doer: doer}, nil
}

var _ HTTPClient = (*Proxied)(nil)

func (p *Proxied) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/enricher")
	return p.doer.Do(ctx, req)
}

func (p *Proxied) Close() error {
	return p.doer.Close()
}