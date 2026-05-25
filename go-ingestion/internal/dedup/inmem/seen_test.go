package inmem

import (
	"context"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
)

func TestMarkSeen(t *testing.T) {
	s := New()
	ctx := context.Background()

	seen, err := s.MarkSeen(ctx, "abc")
	if err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	if seen {
		t.Error("first call should return false (not seen)")
	}

	seen, err = s.MarkSeen(ctx, "abc")
	if err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	if !seen {
		t.Error("second call should return true (already seen)")
	}
}

func TestDistinctKeys(t *testing.T) {
	s := New()
	ctx := context.Background()

	seenA, _ := s.MarkSeen(ctx, "a")
	seenB, _ := s.MarkSeen(ctx, "b")
	seenC, _ := s.MarkSeen(ctx, "c")

	if seenA || seenB || seenC {
		t.Error("first call for each key should return false")
	}

	seenA2, _ := s.MarkSeen(ctx, "a")
	seenB2, _ := s.MarkSeen(ctx, "b")
	seenC2, _ := s.MarkSeen(ctx, "c")

	if !seenA2 || !seenB2 || !seenC2 {
		t.Error("second call for each key should return true (already seen)")
	}
}

var _ dedup.Seen = (*Seen)(nil)
