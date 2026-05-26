package scoring

import (
	"context"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ModelReloader abstracts a reloadable scorer for HTTP health and lifecycle.
// Implementations provide thread-safe hot-reload (e.g. on SIGHUP).
type ModelReloader interface {
	Reload() error
	Version() string
}

// Scorer evaluates feature vectors and produces numeric scores for heroes.
type Scorer interface {
	// Spec returns the feature specification this scorer expects.
	Spec() *domain.FeatureSpec

	// Version returns a unique identifier for this scorer implementation.
	Version() string

	// Score computes a score for each provided feature vector.
	Score(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error)
}
