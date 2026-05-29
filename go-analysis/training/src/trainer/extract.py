"""Extract training data from Postgres.

Training data is filtered to professional competitive matches only:
- leagueid > 0 (matches with an associated league)
- lobby_type IN (1, 2) (practice or tournament lobbies)
"""
import pandas as pd
from sqlalchemy import text
from trainer.config import Settings
from trainer.db import get_engine
from trainer.labels import value_labels

# SQL extracts all pick decisions from matches with a league (professional games).
# Each row = one pick decision with context about which team was acting and outcome.
SQL = text("""
WITH decisions AS (
    SELECT
        m.match_id,
        m.start_time,
        m.patch_id,
        pb.ord AS slot,
        pb.is_pick,
        pb.hero_id,
        CASE WHEN pb.team = 0 THEN m.radiant_team_id
             ELSE m.dire_team_id END AS acting_team,
        CASE WHEN pb.team = 0 THEN m.dire_team_id
             ELSE m.radiant_team_id END AS opp_team,
        (pb.team = 0 AND m.radiant_win) OR
        (pb.team = 1 AND NOT m.radiant_win) AS acting_won
    FROM public.matches m
    JOIN public.picks_bans pb ON pb.match_id = m.match_id
    WHERE m.patch_id = :patch_id
      AND m.leagueid > 0
      AND m.lobby_type IN (1, 2)
)
SELECT * FROM decisions
ORDER BY match_id, slot;
""")


def run(settings: Settings):
    """Extract decisions to Parquet."""
    engine = get_engine(settings)
    df = pd.read_sql(SQL, engine, params={"patch_id": settings.patch_id})

    # Apply value label needed by the value model.
    df = value_labels(df)

    out_path = settings.artifact_dir / "decisions.parquet"
    out_path.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(out_path, index=False)
    print(f"Extracted {len(df)} decisions to {out_path}")
