package matches

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup/inmem"
	metricsinmem "github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics/inmem"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	queueinmem "github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/inmem"
)

type fakeDoer struct {
	mu   sync.Mutex
	body string
	err  error
}

func (f *fakeDoer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

func TestMatchesCycleValidation(t *testing.T) {
	q := queueinmem.New(queue.RetryPolicy{})
	d := &fakeDoer{}
	m := metricsinmem.New()
	
	_, err := New(nil, d, m, Config{})
	if err == nil { t.Error("expected err on nil queue") }

	_, err = New(q, nil, m, Config{})
	if err == nil { t.Error("expected err on nil doer") }

	_, err = New(q, d, m, Config{})
	if err == nil { t.Error("expected err on no queries") }

	cfg := Config{Queries: map[string]string{"foo": "select 1"}}
	_, err = New(q, d, m, cfg)
	if err == nil { t.Error("expected err on missing default query") }

	cfg.DefaultKey = "foo"
	_, err = New(q, d, m, cfg)
	if err != nil { t.Errorf("expected no err, got %v", err) }
}

func TestMatchesCycleRunOnce(t *testing.T) {
	q := queueinmem.New(queue.RetryPolicy{})
	d := &fakeDoer{body: `{"rows": [{"match_ids": [42, "43", 0, "invalid"]}]}`}
	m := metricsinmem.New()
	dd := inmem.New()

	_, _ = dd.MarkSeen(context.Background(), "43") // 43 should be skipped

	cfg := Config{
		Queries: map[string]string{"default": "select 1"},
		ExplorerURL: "http://example.com/api",
		Dedup: dd,
	}

	c, err := New(q, d, m, cfg)
	if err != nil { t.Fatalf("New: %v", err) }

	if c.Name() != "matches" { t.Errorf("Name = %s", c.Name()) }
	if c.Interval() != 0 { t.Errorf("Interval = %v", c.Interval()) }

	ctx := context.Background()
	if err := c.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if q.StreamLen() != 1 {
		t.Errorf("StreamLen = %d, want 1 (42 is pushed, 43 is skipped, 0/invalid ignored)", q.StreamLen())
	}
}

func TestMatchesCycleRunOnceError(t *testing.T) {
	q := queueinmem.New(queue.RetryPolicy{})
	d := &fakeDoer{err: errors.New("boom")}
	m := metricsinmem.New()

	c, _ := New(q, d, m, Config{Queries: map[string]string{"default": "select"}})
	
	if err := c.RunOnce(context.Background()); err == nil {
		t.Error("expected error")
	}
}