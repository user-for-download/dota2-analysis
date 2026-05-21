package inmem

import (
	"context"
	"sync"

	"github.com/user-for-download/go-dota2/internal/dedup"
)

type Seen struct {
	mu   sync.RWMutex
	seen map[string]struct{}
}

func New() *Seen {
	return &Seen{seen: make(map[string]struct{})}
}

var _ dedup.Seen = (*Seen)(nil)

func (s *Seen) MarkSeen(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[key]; ok {
		return true, nil
	}
	s.seen[key] = struct{}{}
	return false, nil
}

func (s *Seen) IsSeen(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.seen[key]
	return ok, nil
}

func (s *Seen) CheckBatch(_ context.Context, keys []string) ([]bool, error) {
	out := make([]bool, len(keys))
	s.mu.RLock()
	for i, k := range keys {
		_, out[i] = s.seen[k]
	}
	s.mu.RUnlock()
	return out, nil
}