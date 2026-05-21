package domain

import (
	"context"
	"time"
)

// RecordedDecision captures a recommendation made during draft analysis.
type RecordedDecision struct {
	MatchID   MatchID
	Slot      int
	Hero      HeroID
	Score     float64
	Reasons   []Reason
	CreatedAt time.Time
}

// DecisionSink persists recorded decisions for later analysis.
type DecisionSink interface {
	Record(ctx context.Context, decision RecordedDecision) error
}

// NoopSink is a DecisionSink that discards all records.
type NoopSink struct{}

// Record implements DecisionSink (no-op).
func (NoopSink) Record(_ context.Context, _ RecordedDecision) error { return nil }
