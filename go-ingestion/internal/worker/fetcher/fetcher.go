package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/payload"
	"github.com/user-for-download/go-dota2/internal/proxy/httpdo"
	"github.com/user-for-download/go-dota2/internal/queue"
	"github.com/user-for-download/go-dota2/internal/worker"
)

type Task struct {
	MatchID int64 `json:"match_id"`
}

type Config struct {
	UpstreamURL string
	PayloadTTL time.Duration
	HTTPTimeout time.Duration
	Batch      int
	Block      time.Duration
	Logger     *slog.Logger
}

type Fetcher struct {
	in    queue.Queue
	out   queue.Queue
	doer  worker.HTTPDoer
	store payload.Store
	m     metrics.Sink
	cfg   Config
	log  *slog.Logger
}

func New(
	in, out queue.Queue,
	doer worker.HTTPDoer,
	store payload.Store,
	m metrics.Sink,
	cfg Config,
) (*Fetcher, error) {
	if in == nil || out == nil {
		return nil, fmt.Errorf("fetcher: in/out queues are required")
	}
	if doer == nil {
		return nil, fmt.Errorf("fetcher: HTTPDoer is required")
	}
	if store == nil {
		return nil, fmt.Errorf("fetcher: payload store is required")
	}
	if m == nil {
		return nil, fmt.Errorf("fetcher: metrics sink is required")
	}
	if cfg.UpstreamURL == "" {
		return nil, fmt.Errorf("fetcher: UpstreamURL is required")
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 10
	}
	if cfg.Block <= 0 {
		cfg.Block = 2 * time.Second
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 15 * time.Second
	}
	if cfg.PayloadTTL <= 0 {
		cfg.PayloadTTL = 1 * time.Hour
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	return &Fetcher{
		in: in, out: out,
		doer: doer, store: store, m: m, cfg: cfg,
		log:  log.With("component", "fetcher"),
	}, nil
}

func (f *Fetcher) Run(ctx context.Context) error {
	return worker.Run(ctx, f.in, worker.Config{
		Batch:      f.cfg.Batch,
		Block:      f.cfg.Block,
		Logger:     f.log,
		RecoverStale: true,
		Metrics:    &latencySink{m: f.m},
	}, metrics.StageFetch, f)
}

var _ worker.Handler = (*Fetcher)(nil)

type latencySink struct{ m metrics.Sink }

func (s *latencySink) RecordLatency(ctx context.Context, stage metrics.Stage, ms float64) {
	s.m.RecordLatency(ctx, stage, ms)
}

func (f *Fetcher) Process(ctx context.Context, t queue.Task) (worker.Result, error) {
	var body Task
	if err := json.Unmarshal(t.Payload, &body); err != nil {
		f.m.FetchFailure(ctx, metrics.KindDecode)
		f.log.Warn("malformed fetch task", "id", t.ID, "err", err)
		return worker.ResultDrop, fmt.Errorf("decode: %w", err)
	}

	blob, err := f.fetchOne(ctx, body.MatchID)
	if err != nil {
		f.m.FetchFailure(ctx, classify(err))
		var perr *httpdo.PermanentHTTPError
		if errors.As(err, &perr) {
			f.log.Info("fetch failed permanently; dropping", "match_id", body.MatchID, "err", err)
			return worker.ResultDrop, err
		}
		f.log.Info("fetch failed; retrying", "match_id", body.MatchID, "err", err)
		return worker.ResultRetry, err
	}

	f.log.Info("fetch ok", "match_id", body.MatchID, "bytes", len(blob))

	key := strconv.FormatInt(body.MatchID, 10)
	if err := f.store.Put(ctx, key, blob, f.cfg.PayloadTTL); err != nil {
		f.m.FetchFailure(ctx, metrics.KindPayload)
		return worker.ResultRetry, fmt.Errorf("payload: %w", err)
	}

	next, _ := json.Marshal(Task{MatchID: body.MatchID})
	if err := f.out.Push(ctx, next); err != nil {
		f.m.FetchFailure(ctx, metrics.KindUnknown)
		_ = f.store.ExtendTTL(ctx, key, 2*time.Hour)
		return worker.ResultRetry, fmt.Errorf("out-queue: %w", err)
	}

	f.m.FetchSuccess(ctx)
	return worker.ResultSuccess, nil
}

func (f *Fetcher) fetchOne(ctx context.Context, matchID int64) ([]byte, error) {
	targetURL := fmt.Sprintf(f.cfg.UpstreamURL, matchID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.doer.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, err
}

func classify(err error) metrics.FailureKind {
	var perr *httpdo.PermanentHTTPError
	if errors.As(err, &perr) {
		code := perr.Code()
		switch code {
		case http.StatusTooManyRequests:
			return metrics.KindRateLimit
		case http.StatusNotFound:
			return metrics.KindNotFound
		}
		return metrics.KindHTTP
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return metrics.KindTimeout
	}
	return metrics.KindUnknown
}