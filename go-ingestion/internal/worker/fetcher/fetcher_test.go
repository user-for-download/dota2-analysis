package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/proxy/httpdo"
	metricsinmem "github.com/user-for-download/go-dota2/internal/metrics/inmem"
	payloadinmem "github.com/user-for-download/go-dota2/internal/payload/inmem"
	"github.com/user-for-download/go-dota2/internal/queue"
	queueinmem "github.com/user-for-download/go-dota2/internal/queue/inmem"
)

// msgFromTask converts a queue.Task to a queue.Message for testing handlers.
func msgFromTask(t queue.Task) queue.Message {
	return queue.Message{ID: t.ID, Payload: t.Payload}
}

type fakeDoer struct {
	mu    sync.Mutex
	calls int
	body  string
	code  int
}

func (f *fakeDoer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	resp := &http.Response{
		StatusCode: f.code,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
		Request:    req,
	}
	if f.code != 200 {
		return resp, &httpdo.PermanentHTTPError{StatusCode: f.code}
	}
	return resp, nil
}

func newFetcher(t *testing.T) (*Fetcher, *queueinmem.Queue, *queueinmem.Queue, *payloadinmem.Store, *metricsinmem.Sink, *fakeDoer) {
	t.Helper()
	in := queueinmem.New(queue.RetryPolicy{MaxRetries: 3, MaxBackoff: time.Second})
	out := queueinmem.New(queue.RetryPolicy{MaxRetries: 3, MaxBackoff: time.Second})
	doer := &fakeDoer{code: 200, body: `{"match_id":42}`}
	store := payloadinmem.New()
	sink := metricsinmem.New()

	f, err := New(in, out, doer, store, sink, Config{
		UpstreamURL: "https://api.example.com/match",
		Batch:       5,
		Block:      50 * time.Millisecond,
		HTTPTimeout: time.Second,
		PayloadTTL:  time.Minute,
		Logger:     slog.Default(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, in, out, store, sink, doer
}

func pushFetchTask(t *testing.T, q queue.Queue, matchID int64) {
	t.Helper()
	b, _ := json.Marshal(Task{MatchID: matchID})
	if err := q.Push(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

func TestFetcherHappyPath(t *testing.T) {
	f, in, out, store, sink, _ := newFetcher(t)

	ctx := context.Background()
	pushFetchTask(t, in, 42)

	tasks, _ := in.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = f.handleMessage(ctx, msgFromTask(tk))
	}

	if store.Len() != 1 {
		t.Errorf("payload not stored: len=%d", store.Len())
	}
	if out.StreamLen() != 1 {
		t.Errorf("out queue len = %d, want 1", out.StreamLen())
	}
	snap, _ := sink.Snapshot(ctx)
	if snap.FetchSuccess != 1 {
		t.Errorf("FetchSuccess = %d, want 1", snap.FetchSuccess)
	}
}

func TestFetcherConfigValidation(t *testing.T) {
	in := queueinmem.New(queue.RetryPolicy{})
	out := queueinmem.New(queue.RetryPolicy{})
	doer := &fakeDoer{}
	store := payloadinmem.New()
	sink := metricsinmem.New()

	_, err := New(nil, out, doer, store, sink, Config{UpstreamURL: "http://example.com/match"})
	if err == nil { t.Error("expected err on nil queueIn") }

	_, err = New(in, nil, doer, store, sink, Config{UpstreamURL: "http://example.com/match"})
	if err == nil { t.Error("expected err on nil queueOut") }

	_, err = New(in, out, nil, store, sink, Config{UpstreamURL: "http://example.com/match"})
	if err == nil { t.Error("expected err on nil doer") }

	_, err = New(in, out, doer, nil, sink, Config{UpstreamURL: "http://example.com/match"})
	if err == nil { t.Error("expected err on nil store") }

	_, err = New(in, out, doer, store, nil, Config{UpstreamURL: "http://example.com/match"})
	if err == nil { t.Error("expected err on nil sink") }

	_, err = New(in, out, doer, store, sink, Config{UpstreamURL: ""})
	if err == nil { t.Error("expected err on empty URL") }

	// Test default config
	_, err = New(in, out, doer, store, sink, Config{UpstreamURL: "http://example.com/match"})
	if err != nil { t.Errorf("expected no err on valid deps, got %v", err) }
}

func TestFetcherRun(t *testing.T) {
	f, _, _, _, _, _ := newFetcher(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := f.Run(ctx)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("expected context error from Run, got %v", err)
	}
}

func TestFetcher403Classified(t *testing.T) {
	f, in, _, _, sink, doer := newFetcher(t)
	doer.code = 403

	ctx := context.Background()
	pushFetchTask(t, in, 42)
	tasks, _ := in.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = f.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindHTTP] != 1 {
		t.Errorf("expected KindHTTP=1 (permanent), got %v", snap.FailuresByKind)
	}
}

func TestFetcher404Classified(t *testing.T) {
	f, in, _, _, sink, doer := newFetcher(t)
	doer.code = 404

	ctx := context.Background()
	pushFetchTask(t, in, 42)
	tasks, _ := in.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = f.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindNotFound] != 1 {
		t.Errorf("expected KindNotFound=1, got %v", snap.FailuresByKind)
	}
}

func TestFetcher429Classified(t *testing.T) {
	f, in, _, _, sink, doer := newFetcher(t)
	doer.code = 429

	ctx := context.Background()
	pushFetchTask(t, in, 42)
	tasks, _ := in.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = f.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindRateLimit] != 1 {
		t.Errorf("expected KindRateLimit=1, got %v", snap.FailuresByKind)
	}
}

func TestFetcherMalformedTaskRouted(t *testing.T) {
	f, in, _, _, sink, _ := newFetcher(t)

	ctx := context.Background()
	if err := in.Push(ctx, []byte("not json")); err != nil {
		t.Fatal(err)
	}
	tasks, _ := in.Pop(ctx, 10, 100*time.Millisecond)
	for _, tk := range tasks {
		_ = f.handleMessage(ctx, msgFromTask(tk))
	}

	snap, _ := sink.Snapshot(ctx)
	if snap.FailuresByKind[metrics.KindDecode] == 0 {
		t.Errorf("expected KindDecode failure")
	}
}
