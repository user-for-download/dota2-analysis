package features

import (
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// FeatureFactory constructs a FeatureSource from shared dependencies.
// Some sources may ignore repo or catalog if they don't need them.
type FeatureFactory func(repo profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error)

// FeatureRegistry maps feature names to their constructors.
// This decouples the ML model's spec.json from the Go compilation boundary:
// the builder reads spec.json, looks up each feature by name, and wires
// sources in the order the model expects — no hardcoded slice in main.go.
type FeatureRegistry struct {
	factories map[string]FeatureFactory
}

// NewFeatureRegistry creates an empty registry.
func NewFeatureRegistry() *FeatureRegistry {
	return &FeatureRegistry{factories: make(map[string]FeatureFactory)}
}

// Register adds a named feature constructor.
func (r *FeatureRegistry) Register(name string, fn FeatureFactory) {
	r.factories[name] = fn
}

// MustRegister registers a feature and panics on duplicate.
func (r *FeatureRegistry) MustRegister(name string, fn FeatureFactory) {
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("feature %q already registered", name))
	}
	r.Register(name, fn)
}

// DefaultRegistry returns a registry pre-populated with all known
// feature sources that ship with this Go binary.
func DefaultRegistry() *FeatureRegistry {
	r := NewFeatureRegistry()

	r.Register("team_picks", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewTeamPicksSource(repo), nil
	})
	r.Register("team_wr_shrunk", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewTeamWRShrunkSource(repo), nil
	})
	r.Register("mean_syn_with_allies", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewSynergySource(repo), nil
	})
	r.Register("mean_counter_vs_enemies", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewCounterSource(repo), nil
	})
	r.Register("hero_meta_primary_attr", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewHeroMetaPrimaryAttrSource(catalog), nil
	})
	r.Register("hero_meta_role_count", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewHeroMetaRoleCountSource(catalog), nil
	})
	r.Register("player_comfort", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewPlayerComfortSource(repo), nil
	})
	r.Register("star_threat", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewStarThreatSource(repo), nil
	})

	return r
}

// NewBuilderFromSpec creates a Builder whose source list and ordering are
// driven entirely by the given FeatureSpec.  This allows the ML model's
// spec.json to determine which features are computed and in what order,
// without requiring a Go recompile when the feature set changes.
//
// Returns an error if any feature named in the spec is not registered.
func NewBuilderFromSpec(spec *domain.FeatureSpec, repo profiles.Repository, catalog domain.HeroCatalog, reg *FeatureRegistry) (Builder, error) {
	if reg == nil {
		return nil, fmt.Errorf("feature registry is nil")
	}
	sources := make([]FeatureSource, len(spec.Features))
	for i, def := range spec.Features {
		fn, ok := reg.factories[def.Name]
		if !ok {
			return nil, fmt.Errorf("feature %q: no registered constructor (available: %s)",
				def.Name, registeredNames(reg))
		}
		src, err := fn(repo, catalog)
		if err != nil {
			return nil, fmt.Errorf("feature %q: construct: %w", def.Name, err)
		}
		sources[i] = src
	}
	return NewBuilder(sources), nil
}

// DefaultSources returns the hardcoded default feature source ordering used
// when no spec.json is available (linear path or LGBM fallback).  These match
// the registered names in DefaultRegistry and keep the same order used by the
// original trained models.
func DefaultSources(repo profiles.Repository, catalog domain.HeroCatalog) []FeatureSource {
	return []FeatureSource{
		NewTeamPicksSource(repo),
		NewTeamWRShrunkSource(repo),
		NewSynergySource(repo),
		NewCounterSource(repo),
		NewHeroMetaPrimaryAttrSource(catalog),
		NewHeroMetaRoleCountSource(catalog),
		NewPlayerComfortSource(repo),
		NewStarThreatSource(repo),
	}
}

// registeredNames returns a comma-separated list of registered feature names.
func registeredNames(reg *FeatureRegistry) string {
	names := make([]string, 0, len(reg.factories))
	for n := range reg.factories {
		names = append(names, n)
	}
	sep := ""
	var out string
	for _, n := range names {
		out += sep + n
		sep = ", "
	}
	return out
}
