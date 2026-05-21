"""Feature spec — single source of truth, must match Go FeatureDefs()."""

# Must match internal/features/specs.go:FeatureSpecVersion
FEATURE_SPEC_VERSION = "2025-11-15"

# Must match internal/features/specs.go:FeatureDefs() — same names, dtypes, order.
# source_hash stores the human-readable description (Go stores the SHA-256[:8] of this).
FEATURES = [
    {"name": "team_picks", "dtype": "f32", "source_hash": "team_picks: SELECT games FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?"},
    {"name": "team_wr_shrunk", "dtype": "f32", "source_hash": "team_wr_shrunk: SELECT wr_shrunk FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?"},
    {"name": "mean_syn_with_allies", "dtype": "f32", "source_hash": "mean_syn: AVG(wr_shrunk) FROM mv_hero_synergy WHERE hero_a IN (allies) AND hero_b=candidate"},
    {"name": "mean_counter_vs_enemies", "dtype": "f32", "source_hash": "mean_counter: AVG(wr_shrunk) FROM mv_hero_counter WHERE hero_a=candidate AND hero_b IN (enemies)"},
    {"name": "hero_meta_primary_attr", "dtype": "f32", "source_hash": "primary_attr: str=1, agi=2, int=3, all=4"},
    {"name": "hero_meta_role_count", "dtype": "f32", "source_hash": "role_count: len(hero.Roles)"},
    {"name": "player_comfort", "dtype": "f32", "source_hash": "player_comfort: wr_shrunk FROM mv_player_hero_profile WHERE account_id=? AND hero_id=?"},
    {"name": "star_threat", "dtype": "f32", "source_hash": "star_threat: opponent signature hero threat level"},
]
