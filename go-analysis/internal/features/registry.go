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

	// ── Per-candidate hero priors ────────────────────────────────────
	r.Register("hero_pick_rate", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewHeroPickRateSource(repo), nil
	})
	r.Register("hero_wr", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewHeroWRSource(repo), nil
	})
	r.Register("hero_popularity", func(repo profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewHeroPopularitySource(repo), nil
	})

	// ── Attribute-based draft features ──────────────────────────────
	r.Register("attr_is_str", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewAttrIsStrSource(catalog), nil
	})
	r.Register("attr_is_agi", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewAttrIsAgiSource(catalog), nil
	})
	r.Register("attr_is_int", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewAttrIsIntSource(catalog), nil
	})
	r.Register("attr_fit_score", func(_ profiles.Repository, catalog domain.HeroCatalog) (FeatureSource, error) {
		return NewAttrFitScoreSource(catalog), nil
	})

	// ── Draft position (same within group) ──────────────────────────
	r.Register("draft_slot_norm", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewDraftSlotNormSource(), nil
	})
	r.Register("is_pick_phase", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewIsPickPhaseSource(), nil
	})

	// ── Semantic draft context (patch-invariant relative state) ─────
	r.Register("team_picks_before", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewTeamPicksBeforeSource(), nil
	})
	r.Register("enemy_picks_before", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewEnemyPicksBeforeSource(), nil
	})
	r.Register("is_first_pick", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewIsFirstPickSource(), nil
	})
	r.Register("is_last_pick", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewIsLastPickSource(), nil
	})
	r.Register("is_counter_phase", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewIsCounterPhaseSource(), nil
	})
	r.Register("remaining_team_picks", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewRemainingTeamPicksSource(), nil
	})
	r.Register("draft_progress", func(_ profiles.Repository, _ domain.HeroCatalog) (FeatureSource, error) {
		return NewDraftProgressSource(), nil
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
// when no spec.json is available (linear path or LGBM fallback).  The order
// must match FEATURES in the training pipeline's feature_specs.py.
func DefaultSources(repo profiles.Repository, catalog domain.HeroCatalog) []FeatureSource {
	return []FeatureSource{
		// MV-dependent (0-7)
		NewTeamPicksSource(repo),
		NewTeamWRShrunkSource(repo),
		NewSynergySource(repo),
		NewCounterSource(repo),
		NewHeroMetaPrimaryAttrSource(catalog),
		NewHeroMetaRoleCountSource(catalog),
		NewPlayerComfortSource(repo),
		NewStarThreatSource(repo),
		// Hero priors (8-10) — vary per candidate
		NewHeroPickRateSource(repo),
		NewHeroWRSource(repo),
		NewHeroPopularitySource(repo),
		// Attribute diversity (11-14) — vary per candidate
		NewAttrIsStrSource(catalog),
		NewAttrIsAgiSource(catalog),
		NewAttrIsIntSource(catalog),
		NewAttrFitScoreSource(catalog),
		// Draft position (15-16) — same within group
		NewDraftSlotNormSource(),
		NewIsPickPhaseSource(),
		// Semantic draft context (17-23) — patch-invariant relative state
		NewTeamPicksBeforeSource(),
		NewEnemyPicksBeforeSource(),
		NewIsFirstPickSource(),
		NewIsLastPickSource(),
		NewIsCounterPhaseSource(),
		NewRemainingTeamPicksSource(),
		NewDraftProgressSource(),
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
