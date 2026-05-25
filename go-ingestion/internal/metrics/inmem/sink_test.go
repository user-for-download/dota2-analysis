package inmem

import (
	"context"
	"errors"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
)

func TestCountersAccumulate(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.IngestSuccess(ctx)
	s.IngestSuccess(ctx)
	s.IngestFailure(ctx, metrics.KindDB, 42, errors.New("boom"))
	s.ParseFailure(ctx, metrics.KindDecode)
	s.FetchSuccess(ctx)
	s.FetchFailure(ctx, metrics.KindHTTP)

	snap, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.IngestSuccess != 2 {
		t.Errorf("IngestSuccess = %d, want 2", snap.IngestSuccess)
	}
	if snap.IngestFailure != 1 {
		t.Errorf("IngestFailure = %d, want 1", snap.IngestFailure)
	}
	if snap.ParseFailure != 1 {
		t.Errorf("ParseFailure = %d, want 1", snap.ParseFailure)
	}
	if snap.FetchSuccess != 1 {
		t.Errorf("FetchSuccess = %d, want 1", snap.FetchSuccess)
	}
	if snap.FetchFailure != 1 {
		t.Errorf("FetchFailure = %d, want 1", snap.FetchFailure)
	}
	if snap.FailuresByKind[metrics.KindDB] != 1 {
		t.Errorf("FailuresByKind[db] = %d, want 1", snap.FailuresByKind[metrics.KindDB])
	}
	if snap.FailuresByKind[metrics.KindDecode] != 1 {
		t.Errorf("FailuresByKind[decode] = %d, want 1", snap.FailuresByKind[metrics.KindDecode])
	}
	if snap.FailuresByKind[metrics.KindHTTP] != 1 {
		t.Errorf("FailuresByKind[http] = %d, want 1", snap.FailuresByKind[metrics.KindHTTP])
	}
}

func TestRecentFailuresCapped(t *testing.T) {
	s := New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.IngestFailure(ctx, metrics.KindDB, int64(i), errors.New("e"))
	}

	snap, _ := s.Snapshot(ctx)
	if len(snap.RecentFailures) != 5 {
		t.Errorf("len(RecentFailures) = %d, want 5", len(snap.RecentFailures))
	}
}

func TestRecentFailuresDisabled(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.IngestFailure(ctx, metrics.KindDB, 1, errors.New("e"))
	snap, _ := s.Snapshot(ctx)

	if len(snap.RecentFailures) != 1 {
		t.Errorf("expected retained failures, got %d", len(snap.RecentFailures))
	}
	if snap.IngestFailure != 1 {
		t.Errorf("counter should still increment: got %d", snap.IngestFailure)
	}
}

func TestSnapshotIsDecoupled(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.IngestFailure(ctx, metrics.KindDB, 1, nil)
	snap, _ := s.Snapshot(ctx)

	snap.FailuresByKind[metrics.KindDB] = 999
	snap.RecentFailures[0].MatchID = -1

	snap2, _ := s.Snapshot(ctx)
	if snap2.FailuresByKind[metrics.KindDB] != 1 {
		t.Error("Snapshot must return a defensive copy of FailuresByKind")
	}
	if snap2.RecentFailures[0].MatchID != 1 {
		t.Error("Snapshot must return a defensive copy of RecentFailures")
	}
}
