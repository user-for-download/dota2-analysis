package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/user-for-download/go-dota2/internal/config"
	"github.com/user-for-download/go-dota2/internal/proxy"
	"github.com/user-for-download/go-dota2/internal/proxy/httpdo"
	"github.com/user-for-download/go-dota2/internal/worker/fetcher"
)

type FetcherDeps struct {
	Worker   *fetcher.Fetcher
	httpDoer io.Closer
}

func (d *FetcherDeps) Close() error {
	if d.httpDoer != nil {
		return d.httpDoer.Close()
	}
	return nil
}

func BuildFetcher(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	core *Deps,
	pool proxy.Pool,
) (*FetcherDeps, error) {
	if !cfg.Fetcher.AllowDirect {
		if err := WaitForProxies(ctx, pool, WaitConfig{
			MinSize:      cfg.Proxy.MinPoolSize,
			Timeout:     cfg.Fetcher.WaitTimeout,
			PollInterval: 2 * time.Second,
		}, log); err != nil {
			return nil, fmt.Errorf("fetcher: proxies not ready: %w", err)
		}
	}

	store, err := PayloadStore(core.Redis.Master(), cfg.Payload)
	if err != nil {
		return nil, fmt.Errorf("fetcher: payload store: %w", err)
	}

	sub, err := FetchQueue(core.Redis.Master(), cfg.Queue, log)
	if err != nil {
		return nil, fmt.Errorf("fetcher: fetch queue: %w", err)
	}

	parseQ, err := ParseQueue(core.Redis.Master(), cfg.Queue, log)
	if err != nil {
		return nil, fmt.Errorf("fetcher: parse queue: %w", err)
	}

	doer, err := httpdo.New(httpdo.Config{
		Pool:        pool,
		Hold:        cfg.Proxy.Hold,
		Timeout:     cfg.Fetcher.HTTPTimeout,
		MaxRetries:  cfg.Fetcher.MaxProxyRetries,
		Backoff:     cfg.Fetcher.ProxyBackoff,
		AllowDirect: cfg.Fetcher.AllowDirect,
		Logger:      log,
	})
	if err != nil {
		return nil, fmt.Errorf("fetcher: httpdoer: %w", err)
	}

	w, err := fetcher.New(sub, parseQ, doer, store, core.Metrics, fetcher.Config{
		UpstreamURL: cfg.Fetcher.UpstreamURL,
		Batch:      cfg.Fetcher.Batch,
		Block:      cfg.Fetcher.Block,
		HTTPTimeout: cfg.Fetcher.HTTPTimeout,
		PayloadTTL:  cfg.Fetcher.PayloadTTL,
		Logger:     log,
	})
	if err != nil {
		doer.Close()
		return nil, fmt.Errorf("fetcher: init: %w", err)
	}

	return &FetcherDeps{Worker: w, httpDoer: doer}, nil
}