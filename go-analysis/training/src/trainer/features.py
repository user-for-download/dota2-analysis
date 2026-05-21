"""Feature engineering on extracted data."""
import pandas as pd
from trainer.config import Settings
from trainer.db import get_engine


def compute_features(decisions: pd.DataFrame, settings: Settings) -> pd.DataFrame:
    """Compute features for each decision.

    This is the simplified pass-through version. The full implementation
    joins decisions with materialized views (mv_team_hero_profile,
    mv_hero_synergy, mv_hero_counter, mv_player_hero_profile) to produce
    the 8-feature vector defined in feature_specs.py.

    Feature order must match FEATURES in feature_specs.py exactly:
      0. team_picks
      1. team_wr_shrunk
      2. mean_syn_with_allies
      3. mean_counter_vs_enemies
      4. hero_meta_primary_attr
      5. hero_meta_role_count
      6. player_comfort
      7. star_threat
    """
    # TODO: Join with MVs for full feature computation.
    # For now, pass through the decision data as-is.
    return decisions.copy()
