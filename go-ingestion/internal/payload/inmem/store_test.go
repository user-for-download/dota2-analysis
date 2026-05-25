package inmem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
)

func TestPutGetDelete(t *testing.T) {
	s := New()
	ctx := context.Background()

	if err := s.Put(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	b, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(b) != "v" {
		t.Errorf("Get = %q, want %q", b, "v")
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "k"); !errors.Is(err, payload.ErrNotFound) {
		t.Errorf("after Delete, err = %v, want ErrNotFound", err)
	}
}

func TestGetMissing(t *testing.T) {
	s := New()
	if _, err := s.Get(context.Background(), "nope"); !errors.Is(err, payload.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestTTLExpires(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.Put(ctx, "k", []byte("v"), 10*time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	if _, err := s.Get(ctx, "k"); !errors.Is(err, payload.ErrNotFound) {
		t.Errorf("expired key should be not found, got %v", err)
	}
}

func TestDefensiveCopy(t *testing.T) {
	s := New()
	ctx := context.Background()

	src := []byte("original")
	_ = s.Put(ctx, "k", src, time.Minute)
	src[0] = 'X'

	got, _ := s.Get(ctx, "k")
	if string(got) != "original" {
		t.Errorf("store must defensively copy on Put; got %q", got)
	}
	got[0] = 'Y'
	got2, _ := s.Get(ctx, "k")
	if string(got2) != "original" {
		t.Errorf("store must defensively copy on Get; got %q", got2)
	}
}
