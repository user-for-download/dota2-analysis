package inmem

import (
	"context"
	"sync"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
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

func (s *Seen) CheckBatch(_ context.Context, keys []string) (map[string]bool, error) {
	result := make(map[string]bool, len(keys))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range keys {
		_, ok := s.seen[k]
		result[k] = ok
	}
	return result, nil
}