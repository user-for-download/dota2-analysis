package redisstreams

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/user-for-download/go-dota2/internal/queue"
)

func newTestQueue(t *testing.T) (*Queue, *goredis.Client, func()) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run")
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis ping failed: %v", err)
	}

	stamp := time.Now().Format("150405.000000000")
	stream := "test:queue:" + stamp
	dlq := "test:queue:dlq:" + stamp

	q, err := New(rdb, Config{
		Stream:      stream,
		DLQStream:   dlq,
		Group:       "g1",
		Consumer:    "c1",
		MaxLen:      1000,
		Policy:      queue.RetryPolicy{MaxRetries: 2, MaxBackoff: 10 * time.Millisecond},
		DeleteOnAck: true,
	})
	if err != nil {
		_ = rdb.Close()
		t.Fatalf("New: %v", err)
	}

	cleanup := func() {
		bg := context.Background()
		_ = rdb.Del(bg, stream, dlq).Err()
		_ = rdb.Close()
	}
	return q, rdb, cleanup
}

func TestRedisStreamsRoundTrip(t *testing.T) {
	q, _, cleanup := newTestQueue(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := q.Push(ctx, []byte("hello")); err != nil {
		t.Fatalf("Push: %v", err)
	}

	tasks, err := q.Pop(ctx, 10, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if string(tasks[0].Payload) != "hello" {
		t.Errorf("payload = %q, want hello", tasks[0].Payload)
	}

	if err := q.Ack(ctx, tasks[0].ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

func TestRedisStreamsRetryToDLQ(t *testing.T) {
	q, rdb, cleanup := newTestQueue(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := q.Push(ctx, []byte("doomed")); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		tasks, err := q.Pop(ctx, 1, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("Pop %d: %v", i, err)
		}
		if err := q.Retry(ctx, tasks[0], "test"); err != nil {
			t.Fatalf("Retry %d: %v", i, err)
		}
	}

	n, err := rdb.XLen(ctx, q.cfg.DLQStream).Result()
	if err != nil {
		t.Fatalf("XLEN dlq: %v", err)
	}
	if n != 1 {
		t.Errorf("DLQ size = %d, want 1", n)
	}
}

func TestRedisStreamsRecoverStale(t *testing.T) {
	q, _, cleanup := newTestQueue(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := q.Push(ctx, []byte("stale")); err != nil {
		t.Fatal(err)
	}

	// Pop to put it in the pending state
	tasks, err := q.Pop(ctx, 1, 500*time.Millisecond)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("Pop: %v, tasks=%d", err, len(tasks))
	}

	// Sleep slightly to let it become idle
	time.Sleep(10 * time.Millisecond)

	// Try to recover it with a very short idle threshold
	recovered, err := q.RecoverStale(ctx, 1*time.Millisecond, 10)
	if err != nil {
		t.Fatalf("RecoverStale: %v", err)
	}

	if len(recovered) != 1 {
		t.Fatalf("recovered = %d, want 1", len(recovered))
	}

	if string(recovered[0].Payload) != "stale" {
		t.Errorf("payload = %q", recovered[0].Payload)
	}
}