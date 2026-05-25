package ingester

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/user-for-download/go-dota2/internal/dedup"
	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
	"github.com/user-for-download/go-dota2/internal/worker"
	"github.com/user-for-download/go-dota2/internal/worker/parser"
)

type Config struct {
	Logger *slog.Logger
	Dedup  dedup.Seen
}

type Ingester struct {
	repo  matchstore.MatchWriter
	m     metrics.Sink
	dedup dedup.Seen
	log   *slog.Logger
}

func New(repo matchstore.MatchWriter, m metrics.Sink, cfg Config) (*Ingester, error) {
	if repo == nil {
		return nil, fmt.Errorf("ingester: repo required")
	}
	if m == nil {
		return nil, fmt.Errorf("ingester: metrics sink required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Ingester{
		repo:  repo,
		m:     m,
		dedup: cfg.Dedup,
		log:   log.With("component", "ingester"),
	}, nil
}

var _ parser.Ingester = (*Ingester)(nil)

func (i *Ingester) Ingest(ctx context.Context, m matchstore.Match) error {
	key := strconv.FormatInt(m.MatchID, 10)
	if i.dedup != nil {
		if _, err := i.dedup.MarkSeen(ctx, key); err != nil {
			i.log.Warn("failed to mark match as seen in dedup", "match_id", m.MatchID, "err", err)
		}
	}

	err := i.repo.IngestMatch(ctx, m)
	if err != nil {
		if isDuplicateConstraint(err) {
			i.m.IngestFailure(ctx, metrics.KindValidate, m.MatchID, err)
			return worker.ErrAlreadySeen
		}
		kind := classify(err)
		if kind == metrics.KindValidate {
			ce := ClassifyDBError(err)
			if ce != nil && ce.IsForeignKey() {
				i.log.Warn("foreign key violation", "match_id", m.MatchID, "detail", ce.Detail)
			}
		}
		i.m.IngestFailure(ctx, kind, m.MatchID, err)
		return fmt.Errorf("repo.IngestMatch: %w", err)
	}
	i.m.IngestSuccess(ctx)
	return nil
}

func isDuplicateConstraint(err error) bool {
	ce := ClassifyDBError(err)
	return ce != nil && ce.Code == "23505"
}

func classify(err error) metrics.FailureKind {
	if err == nil {
		return metrics.KindUnknown
	}
	ce := ClassifyDBError(err)
	if ce != nil {
		switch ce.Code {
		case "23505", "23503":
			return metrics.KindValidate
		case "40001", "40P01":
			return metrics.KindDB
		case "57014":
			return metrics.KindTimeout
		}
	}
	return metrics.KindDB
}
