package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-core/domain"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
)

type Task struct {
	MatchID int64 `json:"match_id"`
}

type Ingester interface {
	Ingest(ctx context.Context, m domain.Match) error
}

type Config struct {
	Batch                        int           // kept for backward compat; Subscribe uses queue defaults
	Block                        time.Duration
	PartitionMaintenanceInterval time.Duration
	Logger                       *slog.Logger
}

type Parser struct {
	in       queue.Subscriber
	store    payload.Store
	ingester Ingester
	m        metrics.Sink
	cfg      Config
	log      *slog.Logger
}

func New(
	in queue.Subscriber,
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
	// When using Subscribe the queue manages Pop/Ack/Retry/backpressure/stale
	// recovery internally.  Batch/Block config should be set on the queue's
	// redisstreams.Config.SubscribeBatch/SubscribeBlock at construction time.
	return p.in.Subscribe(ctx, p.handleMessage)
}

// handleMessage implements queue.Handler.  It processes a single parse task,
// retrieves the stored match payload, decodes/validates it, and ingests it.
func (p *Parser) handleMessage(ctx context.Context, msg queue.Message) error {
	p.log.Debug("parser: received message", "msg_id", msg.ID)
	var body Task
	if err := json.Unmarshal(msg.Payload, &body); err != nil {
		p.m.ParseFailure(ctx, metrics.KindDecode)
		p.log.Warn("malformed parse task", "id", msg.ID, "err", err)
		return queue.ErrDrop
	}

	key := strconv.FormatInt(body.MatchID, 10)
	blob, err := p.store.Get(ctx, key)
	if errors.Is(err, payload.ErrNotFound) {
		p.m.ParseFailure(ctx, metrics.KindPayload)
		p.log.Error("payload expired for match; routing to DLQ",
			"match_id", body.MatchID, "key", key)
		return fmt.Errorf("payload expired for match %d", body.MatchID)
	}
	if err != nil {
		p.m.ParseFailure(ctx, metrics.KindPayload)
		return fmt.Errorf("payload get: %w", err)
	}
	p.log.Debug("parser: payload retrieved", "match_id", body.MatchID, "bytes", len(blob))

	m, err := decodeMatch(body.MatchID, blob)
	if err != nil {
		p.m.ParseFailure(ctx, metrics.KindDecode)
		p.log.Warn("failed to decode match payload",
			"match_id", body.MatchID, "err", err)
		return queue.ErrDrop
	}
	if err := validate(m); err != nil {
		p.m.ParseFailure(ctx, metrics.KindValidate)
		return queue.ErrDrop
	}

	p.log.Debug("match decoded successfully",
		"match_id", m.MatchID,
		"is_parsed", m.IsParsed,
		"players", len(m.Players),
		"duration_sec", m.Duration,
		"radiant_win", m.RadiantWin)

	if err := p.ingester.Ingest(ctx, m); err != nil {
		if errors.Is(err, dedup.ErrAlreadySeen) {
			p.m.ParseDuplicate(ctx)
			p.log.Info("match already in database, skipping", "match_id", m.MatchID)
			if delErr := p.store.Delete(ctx, key); delErr != nil {
				p.log.Warn("failed to delete payload on duplicate", "match_id", m.MatchID, "err", delErr)
			}
			return nil
		}
		p.m.ParseFailure(ctx, metrics.KindIngest)
		_ = p.store.ExtendTTL(ctx, key, 2*time.Hour)
		return fmt.Errorf("ingest: %w", err)
	}

	p.m.ParseSuccess(ctx)
	if delErr := p.store.Delete(ctx, key); delErr != nil {
		p.log.Warn("failed to delete payload after success", "match_id", m.MatchID, "err", delErr)
	}
	return nil
}