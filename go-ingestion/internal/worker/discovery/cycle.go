package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Cycle interface {
	Name() string
	Interval() time.Duration
	RunAtStart() bool
	RunOnce(ctx context.Context) error
}

type HTTPDoer interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

type Fetcher[T any] struct {
	name      string
	interval  time.Duration
	runAtStart bool
	doer      HTTPDoer
	upsertFn  func(context.Context, []T) (int, error)
	fetchFn   func(context.Context) ([]T, error)
	log       *slog.Logger
}

func (f *Fetcher[T]) Name() string            { return f.name }
func (f *Fetcher[T]) Interval() time.Duration { return f.interval }
func (f *Fetcher[T]) RunAtStart() bool        { return f.runAtStart }

func (f *Fetcher[T]) RunOnce(ctx context.Context) error {
	items, err := f.fetchFn(ctx)
	if err != nil {
		return fmt.Errorf("%s: fetch: %w", f.name, err)
	}
	if len(items) == 0 {
		return nil
	}
	n, err := f.upsertFn(ctx, items)
	if err != nil {
		return fmt.Errorf("%s: upsert: %w", f.name, err)
	}
	f.log.Info("upserted", "count", n)
	return nil
}

func NewExplorerCycle[T any](
	name string,
	doer HTTPDoer,
	explorerURL string,
	sql string,
	interval time.Duration,
	log *slog.Logger,
	decode func([]json.RawMessage) ([]T, error),
	upsert func(context.Context, []T) (int, error),
) *Fetcher[T] {
	return &Fetcher[T]{
		name:      name,
		interval:  interval,
		runAtStart: true,
		doer:      doer,
		upsertFn:  upsert,
		fetchFn:   makeExplorerFetch(doer, explorerURL, sql, decode),
		log:       log.With("component", "discovery."+name),
	}
}

func makeExplorerFetch[T any](doer HTTPDoer, explorerURL, sql string, decode func([]json.RawMessage) ([]T, error)) func(context.Context) ([]T, error) {
	return func(ctx context.Context) ([]T, error) {
		base := explorerURL
		if i := strings.Index(base, "?sql="); i > 0 {
			base = base[:i]
		}
		u := base + "?sql=" + url.QueryEscape(sql)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "go-dota2/discoverer")
		resp, err := doer.Do(ctx, req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		var r struct {
			Rows []json.RawMessage `json:"rows"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		return decode(r.Rows)
	}
}

func NewHTTPCycle[T any](
	name string,
	doer HTTPDoer,
	targetURL string,
	interval time.Duration,
	log *slog.Logger,
	decode func([]byte) ([]T, error),
	upsert func(context.Context, []T) (int, error),
) *Fetcher[T] {
	return &Fetcher[T]{
		name:      name,
		interval:  interval,
		runAtStart: true,
		doer:      doer,
		upsertFn:  upsert,
		fetchFn:   makeHTTPFetch(doer, targetURL, decode),
		log:       log.With("component", "discovery."+name),
	}
}

func makeHTTPFetch[T any](doer HTTPDoer, targetURL string, decode func([]byte) ([]T, error)) func(context.Context) ([]T, error) {
	return func(ctx context.Context) ([]T, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "go-dota2/discoverer")
		resp, err := doer.Do(ctx, req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return decode(body)
	}
}

var _ Cycle = (*Fetcher[any])(nil)