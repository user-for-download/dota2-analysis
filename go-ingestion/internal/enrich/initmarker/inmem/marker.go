package inmem

import (
	"context"
	"sync"
)

type Marker struct {
	mu   sync.Mutex
	done map[string]bool
}

func New() *Marker {
	return &Marker{done: make(map[string]bool)}
}

func (m *Marker) Done(ctx context.Context, sourceName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.done[sourceName], nil
}

func (m *Marker) MarkDone(ctx context.Context, sourceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done[sourceName] = true
	return nil
}
