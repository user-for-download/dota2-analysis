package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/user-for-download/go-dota2/internal/payload"
)

type entry struct {
	data      []byte
	expiresAt time.Time
}

type Store struct {
	mu sync.Mutex
	m  map[string]entry
}

func New() *Store {
	return &Store{m: make(map[string]entry)}
}

var _ payload.Store = (*Store)(nil)

func (s *Store) Put(_ context.Context, key string, data []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	s.m[key] = entry{
		data:      append([]byte(nil), data...),
		expiresAt: exp,
	}
	return nil
}

func (s *Store) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return nil, payload.ErrNotFound
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(s.m, key)
		return nil, payload.ErrNotFound
	}
	out := make([]byte, len(e.data))
	copy(out, e.data)
	return out, nil
}

func (s *Store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func (s *Store) ExtendTTL(_ context.Context, key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return payload.ErrNotFound
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	e.expiresAt = time.Now().Add(ttl)
	s.m[key] = e
	return nil
}

func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}
