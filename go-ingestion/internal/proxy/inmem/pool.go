package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
)

type Pool struct {
	mu      sync.Mutex
	proxies []string
	cursor  int
	leased  map[string]struct{}
	stats   map[string]*Stats
}

type Stats struct {
	Successes int
	Failures  int
}

func New(initial []string) *Pool {
	return &Pool{
		proxies: append([]string(nil), initial...),
		leased:  make(map[string]struct{}),
		stats:   make(map[string]*Stats),
	}
}

var _ proxy.Pool = (*Pool)(nil)

func (p *Pool) Acquire(ctx context.Context, _ time.Duration) (*proxy.Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.proxies) == 0 {
		return nil, proxy.ErrNoProxy
	}

	for i := 0; i < len(p.proxies); i++ {
		idx := (p.cursor + i) % len(p.proxies)
		url := p.proxies[idx]
		if _, busy := p.leased[url]; busy {
			continue
		}
		p.leased[url] = struct{}{}
		p.cursor = (idx + 1) % len(p.proxies)
		return proxy.NewLease(
			url,
			p.releaseFn(url),
			p.successFn(url),
			p.failureFn(url),
		), nil
	}
	return nil, proxy.ErrNoProxy
}

func (p *Pool) Size(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.proxies), nil
}

func (p *Pool) Replace(ctx context.Context, healthy []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	newProxies := make(map[string]struct{}, len(healthy))
	for _, u := range healthy {
		newProxies[u] = struct{}{}
	}

	for url := range p.leased {
		if _, ok := newProxies[url]; !ok {
			delete(p.leased, url)
		}
	}

	p.proxies = append([]string(nil), healthy...)
	p.cursor = 0
	return nil
}

func (p *Pool) Add(ctx context.Context, healthy []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := make(map[string]struct{}, len(p.proxies))
	for _, u := range p.proxies {
		seen[u] = struct{}{}
	}
	for _, u := range healthy {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		p.proxies = append(p.proxies, u)
	}
	return nil
}

func (p *Pool) StatsFor(url string) Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.stats[url]; ok {
		return *s
	}
	return Stats{}
}

func (p *Pool) LeasedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.leased)
}

func (p *Pool) releaseFn(url string) func(context.Context) error {
	return func(ctx context.Context) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.leased, url)
		return nil
	}
}

func (p *Pool) successFn(url string) func(context.Context) error {
	return func(ctx context.Context) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		s := p.stats[url]
		if s == nil {
			s = &Stats{}
			p.stats[url] = s
		}
		s.Successes++
		return nil
	}
}

func (p *Pool) failureFn(url string) func(context.Context, error) error {
	return func(ctx context.Context, _ error) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		s := p.stats[url]
		if s == nil {
			s = &Stats{}
			p.stats[url] = s
		}
		s.Failures++
		return nil
	}
}
