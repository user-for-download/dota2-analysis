package ingester

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/resilience"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/parser"
)

// ResilientIngester wraps a parser.Ingester with circuit-breaker protection.
// This keeps the base Ingester focused purely on DB error mapping while
// resilience concerns live in a separate, composable layer.
type ResilientIngester struct {
	next parser.Ingester
	cb   *resilience.CircuitBreaker
	log  *slog.Logger
}

func NewResilient(next parser.Ingester, cb *resilience.CircuitBreaker, log *slog.Logger) *ResilientIngester {
	if log == nil {
		log = slog.Default()
	}
	return &ResilientIngester{
		next: next,
		cb:   cb,
		log:  log.With("component", "resilient_ingester"),
	}
}

func (r *ResilientIngester) Ingest(ctx context.Context, m domain.Match) error {
	if r.cb != nil && !r.cb.Allow() {
		r.log.Warn("circuit breaker open; rejecting ingest", "match_id", m.MatchID)
		r.cb.RecordFailure()
		return fmt.Errorf("circuit breaker open")
	}

	err := r.next.Ingest(ctx, m)
	if err != nil {
		if !IsAlreadySeen(err) && r.cb != nil {
			r.cb.RecordFailure()
		}
		return err
	}

	if r.cb != nil {
		r.cb.RecordSuccess()
	}
	return nil
}
