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

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/httpdo"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker"
)

type Task struct {
	MatchID int64 `json:"match_id"`
}

type Config struct {
	UpstreamURL string
	PayloadTTL  time.Duration
	HTTPTimeout time.Duration
	Batch       int // kept for backward compat; Subscribe uses queue defaults
	Block       time.Duration
	Logger      *slog.Logger
}

type Fetcher struct {
	in    queue.Subscriber
	out   queue.Publisher
	doer  worker.HTTPDoer
	store payload.Store
	m     metrics.Sink
	cfg   Config
	log   *slog.Logger
}

func New(
	in queue.Subscriber,
	out queue.Publisher,
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
	// When using Subscribe the queue manages Pop/Ack/Retry/backpressure/stale
	// recovery internally.  Batch/Block config should be set on the queue's
	// redisstreams.Config.SubscribeBatch/SubscribeBlock at construction time.
	return f.in.Subscribe(ctx, f.handleMessage)
}

// handleMessage implements queue.Handler.  It processes a single fetch task,
// retrieves the match payload from the upstream API, stores it, and enqueues
// a parse task.
func (f *Fetcher) handleMessage(ctx context.Context, msg queue.Message) error {
	var body Task
	if err := json.Unmarshal(msg.Payload, &body); err != nil {
		f.m.FetchFailure(ctx, metrics.KindDecode)
		f.log.Warn("malformed fetch task", "id", msg.ID, "err", err)
		return queue.ErrDrop // permanent decode error — drop, don't retry
	}

	blob, err := f.fetchOne(ctx, body.MatchID)
	if err != nil {
		f.m.FetchFailure(ctx, classify(err))
		var perr *httpdo.PermanentHTTPError
		if errors.As(err, &perr) {
			f.log.Info("fetch failed permanently; dropping", "match_id", body.MatchID, "err", err)
			return queue.ErrDrop
		}
		f.log.Info("fetch failed; retrying", "match_id", body.MatchID, "err", err)
		return err
	}

	f.log.Info("fetch ok", "match_id", body.MatchID, "bytes", len(blob))

	key := strconv.FormatInt(body.MatchID, 10)
	if err := f.store.Put(ctx, key, blob, f.cfg.PayloadTTL); err != nil {
		f.m.FetchFailure(ctx, metrics.KindPayload)
		return fmt.Errorf("payload: %w", err)
	}

	next, err := json.Marshal(Task{MatchID: body.MatchID})
	if err != nil {
		f.m.FetchFailure(ctx, metrics.KindDecode)
		return fmt.Errorf("marshal parse task: %w", err)
	}
	if err := f.out.Publish(ctx, queue.Message{Payload: next}); err != nil {
		f.m.FetchFailure(ctx, metrics.KindUnknown)
		_ = f.store.ExtendTTL(ctx, key, 2*time.Hour)
		return fmt.Errorf("out-queue: %w", err)
	}

	f.m.FetchSuccess(ctx)
	return nil
}

func (f *Fetcher) fetchOne(ctx context.Context, matchID int64) ([]byte, error) {
	targetURL := f.cfg.UpstreamURL + "/" + strconv.FormatInt(matchID, 10)

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