package inmem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/user-for-download/go-dota2/internal/queue"
)

func TestPushPopAck(t *testing.T) {
	q := New(queue.RetryPolicy{MaxRetries: 3, MaxBackoff: time.Second})
	ctx := context.Background()

	if err := q.Push(ctx, []byte("hello")); err != nil {
		t.Fatalf("Push: %v", err)
	}

	tasks, err := q.Pop(ctx, 10, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if string(tasks[0].Payload) != "hello" {
		t.Errorf("payload = %q, want %q", tasks[0].Payload, "hello")
	}
	if q.InFlightLen() != 1 {
		t.Errorf("inflight = %d, want 1", q.InFlightLen())
	}

	if err := q.Ack(ctx, tasks[0].ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if q.InFlightLen() != 0 {
		t.Errorf("inflight after ack = %d, want 0", q.InFlightLen())
	}
}

func TestPushDefensiveCopy(t *testing.T) {
	q := New(queue.RetryPolicy{})
	ctx := context.Background()

	buf := []byte("original")
	if err := q.Push(ctx, buf); err != nil {
		t.Fatal(err)
	}
	buf[0] = 'X'

	tasks, _ := q.Pop(ctx, 1, 100*time.Millisecond)
	if string(tasks[0].Payload) != "original" {
		t.Errorf("payload = %q, want %q (queue must defensively copy)", tasks[0].Payload, "original")
	}
}

func TestPopEmpty(t *testing.T) {
	q := New(queue.RetryPolicy{MaxRetries: 3})
	ctx := context.Background()

	_, err := q.Pop(ctx, 1, 50*time.Millisecond)
	if !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestRetryThenDLQ(t *testing.T) {
	q := New(queue.RetryPolicy{MaxRetries: 2, MaxBackoff: time.Second})
	ctx := context.Background()

	if err := q.Push(ctx, []byte("x")); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		tasks, err := q.Pop(ctx, 1, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Pop %d: %v", i, err)
		}
		if err := q.Retry(ctx, tasks[0], "test"); err != nil {
			t.Fatalf("Retry %d: %v", i, err)
		}
	}

	if got := len(q.DLQ()); got != 1 {
		t.Errorf("DLQ size = %d, want 1", got)
	}
	if q.PendingLen() != 0 {
		t.Errorf("pending = %d, want 0", q.PendingLen())
	}
}

func TestPopBatch(t *testing.T) {
	q := New(queue.RetryPolicy{MaxRetries: 3})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := q.Push(ctx, []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}

	tasks, err := q.Pop(ctx, 3, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}
	if q.PendingLen() != 2 {
		t.Errorf("pending = %d, want 2", q.PendingLen())
	}
}
