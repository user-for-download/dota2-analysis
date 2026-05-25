package parser

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
	metricsinmem "github.com/user-for-download/go-dota2/internal/metrics/inmem"
	payloadinmem "github.com/user-for-download/go-dota2/internal/payload/inmem"
	"github.com/user-for-download/go-dota2/internal/queue"
	queueinmem "github.com/user-for-download/go-dota2/internal/queue/inmem"
	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

// msgFromTask converts a queue.Task to a queue.Message for testing handlers.
func msgFromTask(t queue.Task) queue.Message {
	return queue.Message{ID: t.ID, Payload: t.Payload}
}

type fakeIngester struct {
	mu      sync.Mutex
	matches []matchstore.Match
	err     error
}

func (f *fakeIngester) Ingest(_ context.Context, m matchstore.Match) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.matches = append(f.matches, m)
	return nil
}

func newParser(t *testing.T, ing *fakeIngester) (*Parser, *queueinmem.Queue, *payloadinmem.Store, *metricsinmem.Sink) {
	t.Helper()
	q := queueinmem.New(queue.RetryPolicy{MaxRetries: 3, MaxBackoff: time.Second})
	store := payloadinmem.New()
	sink := metricsinmem.New()
	p, err := New(q, store, ing, sink, Config{
		Batch:  5,
		Block:  50 * time.Millisecond,
		Logger: slog.Default(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, q, store, sink
}

func pushTask(t *testing.T, q queue.Queue, matchID int64) {
	t.Helper()
	b, _ := json.Marshal(Task{MatchID: matchID})
	if err := q.Push(context.Background(), b); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestParserConfigValidation(t *testing.T) {
	q := queueinmem.New(queue.RetryPolicy{})
	store := payloadinmem.New()
	sink := metricsinmem.New()
	ing := &fakeIngester{}

	_, err := New(nil, store, ing, sink, Config{})
	if err == nil { t.Error("expected err on nil queue") }

	_, err = New(q, nil, ing, sink, Config{})
	if err == nil { t.Error("expected err on nil store") }

	_, err = New(q, store, nil, sink, Config{})
	if err == nil { t.Error("expected err on nil ingester") }

	_, err = New(q, store, ing, nil, Config{})
	if err == nil { t.Error("expected err on nil sink") }

	_, err = New(q, store, ing, sink, Config{})
	if err != nil { t.Errorf("expected no err on valid deps, got %v", err) }
}

func TestParserRun(t *testing.T) {
	ing := &fakeIngester{}
	p, _, _, _ := newParser(t, ing)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := p.Run(ctx)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("expected context error from Run, got %v", err)
	}
}

func TestParseHappyPath(t *testing.T) {
	ing := &fakeIngester{}
	p, q, store, sink := newParser(t, ing)

	ctx := context.Background()
	blob := []byte(`{"match_id":42, "start_time":1704067200}`)
	if err := store.Put(ctx, "42", blob, time.Minute); err != nil {
		t.Fatal(err)
	}
	pushTask(t, q, 42)

	tasks, _ := q.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = p.handleMessage(ctx, msgFromTask(tk))
	}

	if len(ing.matches) != 1 || ing.matches[0].MatchID != 42 {
		t.Errorf("matches = %+v", ing.matches)
	}
	if store.Len() != 0 {
		t.Errorf("payload not cleaned up: len=%d", store.Len())
	}
	snap, _ := sink.Snapshot(ctx)
	if snap.ParseFailure != 0 {
		t.Errorf("ParseFailure = %d, want 0", snap.ParseFailure)
	}
}

func TestParsePayloadMissing(t *testing.T) {
	ing := &fakeIngester{}
	p, q, _, sink := newParser(t, ing)

	ctx := context.Background()
	pushTask(t, q, 99)

	tasks, _ := q.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = p.handleMessage(ctx, msgFromTask(tk))
	}

	if len(ing.matches) != 0 {
		t.Errorf("expected no ingests, got %d", len(ing.matches))
	}
	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindPayload] == 0 {
		t.Errorf("expected KindPayload failure")
	}
}

func TestParseDecodeFailure(t *testing.T) {
	ing := &fakeIngester{}
	p, q, store, sink := newParser(t, ing)

	ctx := context.Background()
	if err := store.Put(ctx, "7", []byte("not json"), time.Minute); err != nil {
		t.Fatal(err)
	}
	pushTask(t, q, 7)

	tasks, _ := q.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = p.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindDecode] == 0 {
		t.Errorf("expected KindDecode failure, got %v", snap.FailuresByKind)
	}
}

func TestParseValidateFailure(t *testing.T) {
	ing := &fakeIngester{}
	p, q, store, sink := newParser(t, ing)

	ctx := context.Background()
	if err := store.Put(ctx, "1", []byte(`{"match_id":1,"start_time":1704067200,"players":[{"player_slot":99,"hero_id":1}]}`), time.Minute); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(Task{MatchID: 1})
	_ = q.Push(ctx, b)

	tasks, _ := q.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = p.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindValidate] == 0 {
		t.Errorf("expected KindValidate failure, got %v", snap.FailuresByKind)
	}
}

func TestParseIngesterFailureRetries(t *testing.T) {
	ing := &fakeIngester{err: errors.New("boom")}
	p, q, store, _ := newParser(t, ing)

	ctx := context.Background()
	_ = store.Put(ctx, strconv.FormatInt(42, 10), []byte(`{"match_id":42,"start_time":1704067200}`), time.Minute)
	pushTask(t, q, 42)

	tasks, _ := q.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		err := p.handleMessage(ctx, msgFromTask(tk))
		// handler returns error for retry, nil for success/ErrDrop
		if err == nil || errors.Is(err, queue.ErrDrop) {
			t.Errorf("expected retry error, got %v", err)
		}
		// Simulate what Subscribe does: re-enqueue on retry
		_ = q.Push(ctx, tk.Payload)
	}

	if q.StreamLen() != 1 {
		t.Errorf("pending = %d, want 1 (retry)", q.StreamLen())
	}
}
