package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/payload"
	"github.com/user-for-download/go-dota2/internal/queue"
	"github.com/user-for-download/go-dota2/internal/worker"
)

type Task struct {
	MatchID int64 `json:"match_id"`
}

type Ingester interface {
	Ingest(ctx context.Context, m Match) error
}

type Config struct {
	Batch         int
	Block         time.Duration
	Logger        *slog.Logger
	QueueProvider worker.QueueDepthProvider
}

type Parser struct {
	in       queue.Queue
	store    payload.Store
	ingester Ingester
	m        metrics.Sink
	cfg      Config
	log      *slog.Logger
}

func New(
	in queue.Queue,
	store payload.Store,
	ingester Ingester,
	m metrics.Sink,
	cfg Config,
) (*Parser, error) {
	if in == nil {
		return nil, fmt.Errorf("parser: input queue required")
	}
	if store == nil {
		return nil, fmt.Errorf("parser: payload store required")
	}
	if ingester == nil {
		return nil, fmt.Errorf("parser: ingester required")
	}
	if m == nil {
		return nil, fmt.Errorf("parser: metrics sink required")
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 10
	}
	if cfg.Block <= 0 {
		cfg.Block = 2 * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Parser{
		in: in, store: store, ingester: ingester, m: m, cfg: cfg,
		log: log.With("component", "parser"),
	}, nil
}

func (p *Parser) Run(ctx context.Context) error {
	return worker.Run(ctx, p.in, worker.Config{
		Batch:         p.cfg.Batch,
		Block:         p.cfg.Block,
		Logger:        p.log,
		RecoverStale:  true,
		QueueProvider: p.cfg.QueueProvider,
		Metrics:       &latencySink{m: p.m},
	}, metrics.StageParse, p)
}

type latencySink struct{ m metrics.Sink }

func (s *latencySink) RecordLatency(ctx context.Context, stage metrics.Stage, ms float64) {
	s.m.RecordLatency(ctx, stage, ms)
}

var _ worker.Handler = (*Parser)(nil)

func (p *Parser) Process(ctx context.Context, t queue.Task) (worker.Result, error) {
	var body Task
	if err := json.Unmarshal(t.Payload, &body); err != nil {
		p.m.ParseFailure(ctx, metrics.KindDecode)
		p.log.Warn("malformed parse task", "id", t.ID, "err", err)
		return worker.ResultDrop, fmt.Errorf("decode task: %w", err)
	}

	key := strconv.FormatInt(body.MatchID, 10)
	blob, err := p.store.Get(ctx, key)
	if errors.Is(err, payload.ErrNotFound) {
		p.m.ParseFailure(ctx, metrics.KindPayload)
		p.log.Warn("payload expired; dropping task", "match_id", body.MatchID, "key", key)
		return worker.ResultDrop, err
	}
	if err != nil {
		p.m.ParseFailure(ctx, metrics.KindPayload)
		return worker.ResultRetry, fmt.Errorf("payload get: %w", err)
	}

	m, err := decodeMatch(body.MatchID, blob)
	if err != nil {
		p.m.ParseFailure(ctx, metrics.KindDecode)
		return worker.ResultDrop, fmt.Errorf("decode: %w", err)
	}
	if err := validate(m); err != nil {
		p.m.ParseFailure(ctx, metrics.KindValidate)
		return worker.ResultDrop, fmt.Errorf("validate: %w", err)
	}

	if err := p.ingester.Ingest(ctx, m); err != nil {
		if errors.Is(err, worker.ErrAlreadySeen) {
			p.m.ParseDuplicate(ctx)
			p.log.Info("match already in database, skipping", "match_id", m.MatchID)
			_ = p.store.Delete(ctx, key)
			return worker.ResultSuccess, nil
		}
		p.m.ParseFailure(ctx, metrics.KindIngest)
		_ = p.store.ExtendTTL(ctx, key, 2*time.Hour)
		return worker.ResultRetry, fmt.Errorf("ingest: %w", err)
	}

	p.m.ParseSuccess(ctx)
	_ = p.store.Delete(ctx, key)
	return worker.ResultSuccess, nil
}