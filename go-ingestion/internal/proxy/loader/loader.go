package loader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
)

type Config struct {
	Seed       Source
	Remote     Source
	Validate   Validator
	Parallel   int
	ChunkSize  int
	MinPublish int
	Logger     *slog.Logger
}

type Loader struct {
	pool proxy.Pool
	cfg  Config
	log  *slog.Logger
}

func New(pool proxy.Pool, cfg Config) (*Loader, error) {
	if pool == nil {
		return nil, errors.New("loader: pool required")
	}
	if cfg.Seed == nil {
		return nil, errors.New("loader: seed source required")
	}
	if cfg.Validate.CanaryURL == "" {
		return nil, errors.New("loader: canary URL required")
	}
	if cfg.Parallel <= 0 {
		cfg.Parallel = 20
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 100
	}
	if cfg.MinPublish < 1 {
		cfg.MinPublish = 1
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Loader{pool: pool, cfg: cfg, log: log.With("component", "proxy.loader")}, nil
}

func (l *Loader) Run(ctx context.Context) error {
	seed, err := l.cfg.Seed.Load(ctx)
	if err != nil {
		return fmt.Errorf("seed load: %w", err)
	}
	l.log.Info("seed loaded", "count", len(seed))
	if len(seed) == 0 {
		return errors.New("seed list empty")
	}

	seedHealthy, err := l.validateAndPublishChunks(ctx, seed, "seed")
	if err != nil {
		return fmt.Errorf("seed chunk validation: %w", err)
	}
	if len(seedHealthy) == 0 {
		return errors.New("no healthy proxies after chunked validation")
	}
	l.log.Info("seed phase complete", "healthy", len(seedHealthy), "total", len(seed))

	var allHealthy []string
	if l.cfg.Remote == nil {
		allHealthy = seedHealthy
	} else {
		remote, rerr := l.loadRemoteWithFallback(ctx, seedHealthy)
		if rerr != nil {
			l.log.Warn("remote fetch failed; keeping seed-only pool", "err", rerr)
			allHealthy = seedHealthy
		} else {
			fresh := filterOut(remote, toSet(seedHealthy))
			remoteHealthy, _ := l.validateAndPublishChunks(ctx, fresh, "remote")
			allHealthy = append(seedHealthy, remoteHealthy...)
		}
	}

	// Atomically evict proxies that did not pass this validation cycle
	if err := l.pool.Replace(ctx, allHealthy); err != nil {
		l.log.Warn("failed to evict dead proxies via Replace", "err", err)
	}

	size, _ := l.pool.Size(ctx)
	l.log.Info("run complete", "pool_size", size)
	return nil
}

func (l *Loader) validateAndPublishChunks(ctx context.Context, list []string, label string) ([]string, error) {
	if len(list) == 0 {
		return nil, nil
	}

	var allHealthy []string
	chunks := (len(list) + l.cfg.ChunkSize - 1) / l.cfg.ChunkSize

	for idx := 0; idx < chunks; idx++ {
		if err := ctx.Err(); err != nil {
			return allHealthy, err
		}
		start := idx * l.cfg.ChunkSize
		end := start + l.cfg.ChunkSize
		if end > len(list) {
			end = len(list)
		}
		chunk := list[start:end]

		healthy := l.validateAll(ctx, chunk)
		l.log.Info("chunk validated",
			"label", label,
			"chunk", idx+1,
			"of", chunks,
			"size", len(chunk),
			"healthy", len(healthy),
		)

		if len(healthy) > 0 {
			if len(healthy) < l.cfg.MinPublish {
				l.log.Warn("healthy proxies below MinPublish threshold; not publishing to pool",
					"healthy", len(healthy), "min", l.cfg.MinPublish, "label", label)
			} else {
				if err := l.pool.Add(ctx, healthy); err != nil {
					l.log.Warn("pool add failed; chunk not published",
						"err", err, "count", len(healthy))
				} else {
					l.log.Info("chunk published", "count", len(healthy), "label", label)
				}
			}
		} else if idx > 0 {
			l.log.Warn("no healthy proxies in chunk",
				"label", label)
		}
		allHealthy = append(allHealthy, healthy...)
	}
	return allHealthy, nil
}

func (l *Loader) loadRemoteWithFallback(ctx context.Context, candidates []string) ([]string, error) {
	hs, ok := l.cfg.Remote.(HTTPSource)
	if !ok {
		return l.cfg.Remote.Load(ctx)
	}
	if len(candidates) == 0 {
		return nil, errors.New("no candidates to fetch remote list through")
	}

	max := len(candidates)
	if max > 5 {
		max = 5
	}
	var lastErr error
	for i := 0; i < max; i++ {
		attempt := hs
		attempt.ProxyURL = candidates[i]
		out, err := attempt.Load(ctx)
		if err == nil {
			return out, nil
		}
		lastErr = err
		l.log.Debug("remote fetch attempt failed",
			"proxy", candidates[i], "attempt", i+1, "err", err)
	}
	return nil, fmt.Errorf("remote fetch: %d/%d attempts failed, last: %w",
		max, len(candidates), lastErr)
}

func (l *Loader) validateAll(ctx context.Context, list []string) []string {
	sem := make(chan struct{}, l.cfg.Parallel)
	var (
		mu      sync.Mutex
		healthy []string
		wg      sync.WaitGroup
	)

	for _, p := range list {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(proxyURL string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}
			if err := l.cfg.Validate.Probe(ctx, proxyURL); err != nil {
				l.log.Debug("probe failed", "proxy", proxyURL, "err", err)
				return
			}
			mu.Lock()
			healthy = append(healthy, proxyURL)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return healthy
}

func toSet(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, x := range s {
		m[x] = struct{}{}
	}
	return m
}

func filterOut(in []string, drop map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for _, x := range in {
		if _, ok := drop[x]; ok {
			continue
		}
		out = append(out, x)
	}
	return out
}