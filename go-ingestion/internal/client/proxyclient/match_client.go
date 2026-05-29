package proxyclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/client"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/httpdo"
)

type Config struct {
	Pool        proxy.Pool
	UpstreamURL string
	Hold        time.Duration // proxy lease TTL (default 30s)
	Timeout     time.Duration
	MaxRetries  int
	Backoff     time.Duration
	AllowDirect bool
	Logger      *slog.Logger
}

type MatchClient struct {
	cfg     Config
	log     *slog.Logger
	doer    *httpdo.Doer
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
	if cfg.Hold <= 0 {
		cfg.Hold = 30 * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

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
		return nil, err
	}

	return &MatchClient{
		cfg:     cfg,
		log:     log.With("component", "proxyclient.match"),
		doer:    doer,
		baseURL: cfg.UpstreamURL,
	}, nil
}

var _ client.MatchClient = (*MatchClient)(nil)

func (c *MatchClient) GetMatch(ctx context.Context, matchID int64) ([]byte, error) {
	targetURL := c.baseURL + "/" + strconv.FormatInt(matchID, 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/match-client")

	resp, err := c.doer.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// Close releases resources held by the underlying httpdo.Doer.
func (c *MatchClient) Close() error {
	return c.doer.Close()
}
