"""Extract training data from Postgres."""
import pandas as pd
from sqlalchemy import text
from trainer.config import Settings
from trainer.db import get_engine
from trainer.labels import value_labels

# Template: {limit_clause} is injected at runtime from settings.extract_limit.
SQL_TEMPLATE = """
WITH decisions AS (
    SELECT
        m.match_id,
        m.start_time,
        m.patch_id,
        pb.ord AS slot,
        pb.is_pick,
        pb.hero_id,
        pb.team,
        CASE WHEN pb.team = 0 THEN m.radiant_team_id
             ELSE m.dire_team_id END AS acting_team,
        CASE WHEN pb.team = 0 THEN m.dire_team_id
             ELSE m.radiant_team_id END AS opp_team,
        (pb.team = 0 AND m.radiant_win) OR
        (pb.team = 1 AND NOT m.radiant_win) AS acting_won
    FROM public.matches m
    JOIN public.picks_bans pb ON pb.match_id = m.match_id
    WHERE m.patch_id = ANY(:patch_ids)
      AND m.leagueid > 0
      AND m.lobby_type IN (1, 2)
)
SELECT * FROM decisions
ORDER BY match_id, slot
{limit_clause}
"""


def get_patch_ids(engine, patch_id: int, depth: int) -> list[int]:
    """Fetch a list of patch IDs ending at patch_id, going back 'depth' patches."""
    if depth <= 1:
        return [patch_id]

    sql = text("""
        SELECT id FROM patches
        WHERE id <= :patch_id
        ORDER BY id DESC
        LIMIT :depth
    """)
    with engine.connect() as conn:
        result = conn.execute(sql, {"patch_id": patch_id, "depth": depth})
        ids = [row[0] for row in result]

    if not ids:
        print(f"WARNING: No patches found for id <= {patch_id}. Falling back to single patch.")
        return [patch_id]

    return ids


def run(settings: Settings):
    """Extract decisions to Parquet."""
    engine = get_engine(settings)

    # Resolve the group of patches
    patch_ids = get_patch_ids(engine, settings.patch_id, settings.depth_patch)
    print(f"Training on {len(patch_ids)} patch(es): {patch_ids}")

    # Dynamically build LIMIT clause (empty string if 0 = no limit).
    limit_clause = f"LIMIT {settings.extract_limit}" if settings.extract_limit > 0 else ""
    sql = text(SQL_TEMPLATE.format(limit_clause=limit_clause))

    # psycopg2 automatically adapts Python lists to Postgres arrays for ANY()
    df = pd.read_sql(sql, engine, params={"patch_ids": patch_ids})

    # Apply value label needed by the value model.
    df = value_labels(df)

    out_path = settings.artifact_dir / "decisions.parquet"
    out_path.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(out_path, index=False)
    print(f"Extracted {len(df)} decisions to {out_path}")
