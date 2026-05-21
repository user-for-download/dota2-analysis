package ingester

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/user-for-download/go-dota2/internal/metrics"
	metricsinmem "github.com/user-for-download/go-dota2/internal/metrics/inmem"
	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

type fakeRepo struct {
	mu      sync.Mutex
	ingests []matchstore.Match
	err     error
}

func (r *fakeRepo) IngestMatch(_ context.Context, m matchstore.Match) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.ingests = append(r.ingests, m)
	return nil
}

func newIng(t *testing.T, repo matchstore.MatchWriter) (*Ingester, *metricsinmem.Sink) {
	t.Helper()
	sink := metricsinmem.New()
	i, err := New(repo, sink, Config{Logger: slog.Default()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return i, sink
}

func TestIngesterConfigValidation(t *testing.T) {
	repo := &fakeRepo{}
	sink := metricsinmem.New()

	if _, err := New(nil, sink, Config{}); err == nil {
		t.Error("expected error on nil repo")
	}
	if _, err := New(repo, nil, Config{}); err == nil {
		t.Error("expected error on nil sink")
	}
	if _, err := New(repo, sink, Config{}); err != nil {
		t.Errorf("expected no err, got %v", err)
	}
}

func TestIngestHappyPath(t *testing.T) {
	repo := &fakeRepo{}
	i, sink := newIng(t, repo)

	err := i.Ingest(context.Background(), matchstore.Match{MatchID: 42})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(repo.ingests) != 1 {
		t.Errorf("ingests = %d, want 1", len(repo.ingests))
	}
	snap, _ := sink.Snapshot(context.Background())
	if snap.IngestSuccess != 1 {
		t.Errorf("IngestSuccess = %d, want 1", snap.IngestSuccess)
	}
}

func TestIngestIdempotent(t *testing.T) {
	repo := &fakeRepo{}
	i, sink := newIng(t, repo)

	ctx := context.Background()
	_ = i.Ingest(ctx, matchstore.Match{MatchID: 42})
	_ = i.Ingest(ctx, matchstore.Match{MatchID: 42})

	if len(repo.ingests) != 2 {
		t.Errorf("ingests = %d, want 2", len(repo.ingests))
	}
	snap, _ := sink.Snapshot(ctx)
	if snap.IngestSuccess != 2 {
		t.Errorf("IngestSuccess = %d, want 2", snap.IngestSuccess)
	}
}

func TestIngestRepoFailure(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	i, sink := newIng(t, repo)

	err := i.Ingest(context.Background(), matchstore.Match{MatchID: 42})
	if err == nil {
		t.Fatal("expected error")
	}
	snap, _ := sink.Snapshot(context.Background())
	if snap.IngestFailure != 1 {
		t.Errorf("IngestFailure = %d, want 1", snap.IngestFailure)
	}
	if snap.FailuresByKind[metrics.KindDB] != 1 {
		t.Errorf("FailuresByKind[db] = %d, want 1", snap.FailuresByKind[metrics.KindDB])
	}
}

func TestClassify(t *testing.T) {
	if classify(nil) != metrics.KindUnknown {
		t.Error("expected KindUnknown on nil error")
	}
	if classify(errors.New("random")) != metrics.KindDB {
		t.Error("expected KindDB on unknown error")
	}

	cases := []struct {
		code string
		want metrics.FailureKind
	}{
		{"23505", metrics.KindValidate},
		{"23503", metrics.KindValidate},
		{"40001", metrics.KindDB},
		{"40P01", metrics.KindDB},
		{"57014", metrics.KindTimeout},
		{"99999", metrics.KindDB},
	}

	for _, c := range cases {
		err := &pgconn.PgError{Code: c.code}
		if got := classify(err); got != c.want {
			t.Errorf("classify(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestIngestForeignKeyViolation(t *testing.T) {
	repo := &fakeRepo{err: &pgconn.PgError{Code: "23503", Detail: "fk error"}}
	i, sink := newIng(t, repo)

	err := i.Ingest(context.Background(), matchstore.Match{MatchID: 42})
	if err == nil {
		t.Fatal("expected error")
	}
	snap, _ := sink.Snapshot(context.Background())
	if snap.FailuresByKind[metrics.KindValidate] != 1 {
		t.Errorf("FailuresByKind[validate] = %d, want 1", snap.FailuresByKind[metrics.KindValidate])
	}
}