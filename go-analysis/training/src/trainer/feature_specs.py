"""Feature spec — single source of truth, must match Go FeatureDefs().

Design principle: features must VARY ACROSS CANDIDATES within a decision
group for LambdaMART to learn a ranking.  Features 0-7 are MV-dependent
and often constant when MVs are empty — rely on features 8-14 for ranking
signal during cold-start training.
"""

# Must match internal/features/specs.go:FeatureSpecVersion
FEATURE_SPEC_VERSION = "2026-05-26"

# Must match internal/features/specs.go:FeatureDefs() — same names, dtypes, order.
# source_hash stores the human-readable description (Go stores the SHA-256[:8] of this).
FEATURES = [
    # ── MV-dependent features (constant across candidates when MVs empty) ──
    {"name": "team_picks", "dtype": "f32", "source_hash": "team_picks: SELECT games FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?"},
    {"name": "team_wr_shrunk", "dtype": "f32", "source_hash": "team_wr_shrunk: SELECT wr_shrunk FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?"},
    {"name": "mean_syn_with_allies", "dtype": "f32", "source_hash": "mean_syn: AVG(wr_shrunk) FROM mv_hero_synergy WHERE hero_a IN (allies) AND hero_b=candidate"},
    {"name": "mean_counter_vs_enemies", "dtype": "f32", "source_hash": "mean_counter: AVG(wr_shrunk) FROM mv_hero_counter WHERE hero_a=candidate AND hero_b IN (enemies)"},
    {"name": "hero_meta_primary_attr", "dtype": "f32", "source_hash": "primary_attr: str=1, agi=2, int=3, all=4"},
    {"name": "hero_meta_role_count", "dtype": "f32", "source_hash": "role_count: len(hero.Roles)"},
    {"name": "player_comfort", "dtype": "f32", "source_hash": "player_comfort: wr_shrunk FROM mv_player_hero_profile WHERE account_id=? AND hero_id=?"},
    {"name": "star_threat", "dtype": "f32", "source_hash": "star_threat: opponent signature hero threat level"},
    # ── Per-candidate hero priors (VARY per hero — primary ranking signal) ──
    {"name": "hero_pick_rate", "dtype": "f32", "source_hash": "hero_pick_rate: shrunk pick freq from full corpus (not label leakage with 31 cand/dec)"},
    {"name": "hero_wr", "dtype": "f32", "source_hash": "hero_wr: shrunk win rate from full corpus, varies per hero"},
    {"name": "hero_popularity", "dtype": "f32", "source_hash": "hero_popularity: log1p(pick_count) captures long-tail distribution"},
    # ── Attribute-based draft features (VARY per hero — secondary signal) ───
    {"name": "attr_is_str", "dtype": "f32", "source_hash": "attr_is_str: 1.0 for STR heroes (primary_attr=1)"},
    {"name": "attr_is_agi", "dtype": "f32", "source_hash": "attr_is_agi: 1.0 for AGI heroes (primary_attr=2)"},
    {"name": "attr_is_int", "dtype": "f32", "source_hash": "attr_is_int: 1.0 for INT heroes (primary_attr=3)"},
    {"name": "attr_fit_score", "dtype": "f32", "source_hash": "attr_fit_score: team_picks * (is_int*0.5 + is_agi*0.3 + is_str*0.2)"},
    # ── Draft position (same within group — weak group-level signal only) ──
    {"name": "draft_slot_norm", "dtype": "f32", "source_hash": "draft_slot_norm: slot/max_slot normalized to [0,1]"},
    {"name": "is_pick_phase", "dtype": "f32", "source_hash": "is_pick_phase: 1.0 for picks, 0.0 for bans"},
    # ── Semantic draft context (same within group — patch-invariant state) ──
    {"name": "team_picks_before", "dtype": "f32", "source_hash": "team_picks_before: picks by acting_team before this slot"},
    {"name": "enemy_picks_before", "dtype": "f32", "source_hash": "enemy_picks_before: picks by enemy team before this slot"},
    {"name": "is_first_pick", "dtype": "f32", "source_hash": "is_first_pick: team_picks_before == 0"},
    {"name": "is_last_pick", "dtype": "f32", "source_hash": "is_last_pick: team_picks_before == 4 (5th pick)"},
    {"name": "is_counter_phase", "dtype": "f32", "source_hash": "is_counter_phase: enemy_picks_before > team_picks_before"},
    {"name": "remaining_team_picks", "dtype": "f32", "source_hash": "remaining_team_picks: 5 - team_picks_before"},
    {"name": "draft_progress", "dtype": "f32", "source_hash": "draft_progress: (team + enemy picks) / 10"},
]
