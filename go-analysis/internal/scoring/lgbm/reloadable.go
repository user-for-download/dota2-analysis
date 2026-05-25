package lgbm

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ReloadableScorer wraps a Scorer in an atomic.Pointer for thread-safe hot-reload.
type ReloadableScorer struct {
	ptr atomic.Pointer[Scorer]
}

// NewReloadableScorer creates a reloadable scorer from an initial model.
func NewReloadableScorer(scorer *Scorer) *ReloadableScorer {
	rs := &ReloadableScorer{}
	rs.ptr.Store(scorer)
	return rs
}

// Load returns the current scorer.
func (r *ReloadableScorer) Load() *Scorer {
	return r.ptr.Load()
}

// Reload loads a new model and atomically swaps it in.
func (r *ReloadableScorer) Reload() error {
	cur := r.ptr.Load()
	if cur == nil {
		return nil
	}
	newScorer, err := cur.Reload()
	if err != nil {
		return err
	}
	r.ptr.Store(newScorer)
	return nil
}

// Spec returns the current scorer's spec.
func (r *ReloadableScorer) Spec() *domain.FeatureSpec {
	s := r.ptr.Load()
	if s == nil {
		return nil
	}
	return s.Spec()
}

// Version returns the current scorer's version.
func (r *ReloadableScorer) Version() string {
	s := r.ptr.Load()
	if s == nil {
		return ""
	}
	return s.Version()
}

// Score delegates to the current scorer.
func (r *ReloadableScorer) Score(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
	s := r.ptr.Load()
	if s == nil {
		return nil, fmt.Errorf("scorer not loaded")
	}
	return s.Score(ctx, vectors)
}
