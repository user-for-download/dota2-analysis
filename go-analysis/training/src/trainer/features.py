"""Feature engineering on extracted data."""
import pandas as pd
import numpy as np
from sqlalchemy import text
from trainer.config import Settings
from trainer.db import get_engine


def _refresh_mvs(engine):
    """Refresh all analytics materialized views so data is current."""
    mvs = [
        "mv_team_hero_profile",
        "mv_hero_synergy",
        "mv_hero_counter",
        "mv_player_team_history",
        "mv_player_hero_profile",
    ]
    for mv in mvs:
        print(f"  Refreshing analytics.{mv}...")
        engine.execute(text(f"REFRESH MATERIALIZED VIEW analytics.{mv}"))


def _load_mvs(engine) -> dict[str, pd.DataFrame]:
    """Load all materialized views and reference tables into DataFrames."""
    team_hero = pd.read_sql(
        text("""
            SELECT team_id, hero_id,
                   games::int AS team_picks,
                   shrunk_wr AS team_wr_shrunk
            FROM analytics.mv_team_hero_profile
        """),
        engine,
    )

    synergy = pd.read_sql(
        text("SELECT hero_a, hero_b, shrunk_wr FROM analytics.mv_hero_synergy"),
        engine,
    )

    counter = pd.read_sql(
        text("SELECT hero_a, hero_b, shrunk_wr FROM analytics.mv_hero_counter"),
        engine,
    )

    hero_meta = pd.read_sql(
        text("""
            SELECT id,
                   CASE primary_attr
                       WHEN 'str' THEN 1 WHEN 'agi' THEN 2
                       WHEN 'int' THEN 3 WHEN 'all' THEN 4
                       ELSE 0
                   END AS hero_meta_primary_attr,
                   COALESCE(array_length(roles, 1), 0) AS hero_meta_role_count
            FROM public.heroes
        """),
        engine,
    )

    player_hero = pd.read_sql(
        text("""
            SELECT account_id, hero_id,
                   shrunk_wr AS ph_comfort
            FROM analytics.mv_player_hero_profile
        """),
        engine,
    )

    return {
        "team_hero": team_hero,
        "synergy": synergy,
        "counter": counter,
        "hero_meta": hero_meta,
        "player_hero": player_hero,
    }


def _features_team(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame]
) -> pd.DataFrame:
    """Features 0-1: team_picks, team_wr_shrunk."""
    th = mvs["team_hero"]
    df = df.merge(
        th[["team_id", "hero_id", "team_picks", "team_wr_shrunk"]],
        left_on=["acting_team", "hero_id"],
        right_on=["team_id", "hero_id"],
        how="left",
    )
    df["team_picks"] = df["team_picks"].fillna(0).astype(int)
    df["team_wr_shrunk"] = df["team_wr_shrunk"].fillna(0.5)
    df.drop(columns=["team_id"], inplace=True, errors="ignore")
    return df


def _features_synergy(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame]
) -> pd.DataFrame:
    """Feature 2: mean_syn_with_allies.

    For each candidate, average synergy with all actual ally picks
    in the same match.  Synergy MV stores unordered hero pairs
    (hero_a < hero_b).
    """
    syn = mvs["synergy"]
    if syn.empty:
        df["mean_syn_with_allies"] = 0.5
        return df

    # Actual picks (positive samples) define the ally set.
    actual = df[df["label"] == 1.0][["match_id", "hero_id", "acting_team"]].drop_duplicates()
    if actual.empty:
        df["mean_syn_with_allies"] = 0.5
        return df

    # Cross each candidate with all actual picks in the same match.
    cross = df[["match_id", "hero_id"]].drop_duplicates().merge(
        actual.rename(columns={"hero_id": "ally_hero_id"})[["match_id", "ally_hero_id"]],
        on="match_id",
    )
    cross = cross[cross["hero_id"] != cross["ally_hero_id"]]

    # Normalise pair order for synergy lookup.
    cross["a"] = cross[["hero_id", "ally_hero_id"]].min(axis=1)
    cross["b"] = cross[["hero_id", "ally_hero_id"]].max(axis=1)
    cross = cross.merge(syn, left_on=["a", "b"], right_on=["hero_a", "hero_b"], how="left")

    mean_syn = cross.groupby(["match_id", "hero_id"], as_index=False)["shrunk_wr"].mean()
    mean_syn.rename(columns={"shrunk_wr": "mean_syn_with_allies"}, inplace=True)

    df = df.merge(mean_syn, on=["match_id", "hero_id"], how="left")
    df["mean_syn_with_allies"] = df["mean_syn_with_allies"].fillna(0.5)
    return df


def _features_counter(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame]
) -> pd.DataFrame:
    """Feature 3: mean_counter_vs_enemies.

    For each candidate, average counter WR against all actual enemy
    picks in the same match.  Counter MV stores hero_a (the hero whose
    WR is measured) vs hero_b (the opponent).
    """
    cnt = mvs["counter"]
    if cnt.empty:
        df["mean_counter_vs_enemies"] = 0.5
        return df

    # Actual picks with their team perspective.
    actual = df[df["label"] == 1.0][
        ["match_id", "hero_id", "acting_team", "opp_team"]
    ].drop_duplicates()
    if actual.empty:
        df["mean_counter_vs_enemies"] = 0.5
        return df

    # For each actual pick, the *enemy* picks are the rows in the same
    # match where acting_team = this row's opp_team.
    enemy_map = actual.rename(
        columns={"hero_id": "enemy_hero_id", "acting_team": "enemy_acting", "opp_team": "enemy_opp"}
    )[["match_id", "enemy_hero_id", "enemy_acting"]]

    # Cross each candidate with enemy picks in the same match.
    cross = df[["match_id", "hero_id", "acting_team", "opp_team"]].drop_duplicates()

    # Find enemy picks: rows in same match where acting_team = candidate's opp_team
    # We join cross with enemy_map: candidate's opp_team matches enemy's acting_team
    cross = cross.merge(
        enemy_map,
        left_on=["match_id", "opp_team"],
        right_on=["match_id", "enemy_acting"],
        how="inner",
    )

    # Counter: hero_a = candidate, hero_b = enemy.
    cross = cross.merge(
        cnt,
        left_on=["hero_id", "enemy_hero_id"],
        right_on=["hero_a", "hero_b"],
        how="left",
    )

    mean_cnt = cross.groupby(["match_id", "hero_id"], as_index=False)["shrunk_wr"].mean()
    mean_cnt.rename(columns={"shrunk_wr": "mean_counter_vs_enemies"}, inplace=True)

    df = df.merge(mean_cnt, on=["match_id", "hero_id"], how="left")
    df["mean_counter_vs_enemies"] = df["mean_counter_vs_enemies"].fillna(0.5)
    return df


def _features_hero_meta(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame]
) -> pd.DataFrame:
    """Features 4-5: hero_meta_primary_attr, hero_meta_role_count."""
    hm = mvs["hero_meta"]
    if hm.empty:
        df["hero_meta_primary_attr"] = 0
        df["hero_meta_role_count"] = 0
        return df

    df = df.merge(
        hm[["id", "hero_meta_primary_attr", "hero_meta_role_count"]],
        left_on="hero_id",
        right_on="id",
        how="left",
    )
    df["hero_meta_primary_attr"] = df["hero_meta_primary_attr"].fillna(0).astype(int)
    df["hero_meta_role_count"] = df["hero_meta_role_count"].fillna(0).astype(int)
    df.drop(columns=["id"], inplace=True, errors="ignore")
    return df


def _load_roster(engine, match_ids: list[int]) -> pd.DataFrame:
    """Load player roster per match for both teams."""
    if not match_ids:
        return pd.DataFrame(columns=["match_id", "team_id", "account_id"])

    # Load in chunks if too many match_ids for a single IN clause.
    chunk_size = 500
    chunks = []
    for i in range(0, len(match_ids), chunk_size):
        chunk = match_ids[i : i + chunk_size]
        q = text("""
            SELECT pm.match_id, pm.account_id,
                   CASE WHEN pm.is_radiant THEN m.radiant_team_id
                        ELSE m.dire_team_id
                   END AS team_id
            FROM public.player_matches pm
            JOIN public.matches m ON m.match_id = pm.match_id AND m.start_time = pm.start_time
            WHERE pm.match_id = ANY(:mids)
              AND pm.account_id IS NOT NULL
        """)
        chunk_df = pd.read_sql(q, engine, params={"mids": chunk})
        chunks.append(chunk_df)

    if not chunks:
        return pd.DataFrame(columns=["match_id", "team_id", "account_id"])
    return pd.concat(chunks, ignore_index=True)


def _features_player_comfort(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame], engine
) -> pd.DataFrame:
    """Feature 6: player_comfort.

    Average shrunk_wr across the acting team's roster for each
    candidate hero.
    """
    ph = mvs["player_hero"]
    if ph.empty:
        df["player_comfort"] = 0.5
        return df

    match_ids = df["match_id"].unique().tolist()
    roster = _load_roster(engine, match_ids)

    if roster.empty:
        df["player_comfort"] = 0.5
        return df

    # Cross each candidate with the acting team's roster.
    # First get unique player-team per match.
    roster_uq = roster.drop_duplicates(subset=["match_id", "team_id", "account_id"])

    # For each (match_id, acting_team, hero_id), find the roster
    # where team_id = acting_team, then look up each player's comfort
    # for this hero.
    cross = df[["match_id", "acting_team", "hero_id"]].drop_duplicates().merge(
        roster_uq.rename(columns={"team_id": "roster_team"}),
        left_on=["match_id", "acting_team"],
        right_on=["match_id", "roster_team"],
        how="inner",
    )

    # Merge player comfort scores.
    cross = cross.merge(
        ph,
        on=["account_id", "hero_id"],
        how="left",
    )
    cross["ph_comfort"] = cross["ph_comfort"].fillna(0.5)

    # Average across the roster.
    avg_comfort = cross.groupby(["match_id", "hero_id"], as_index=False)["ph_comfort"].mean()
    avg_comfort.rename(columns={"ph_comfort": "player_comfort"}, inplace=True)

    df = df.merge(avg_comfort, on=["match_id", "hero_id"], how="left")
    df["player_comfort"] = df["player_comfort"].fillna(0.5)
    return df


def _features_star_threat(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame], engine
) -> pd.DataFrame:
    """Feature 7: star_threat.

    The opponent's average best-hero comfort across their roster.
    For each opponent player, take their single highest shrunk_wr hero
    (signature hero), then average those values as the threat level.
    """
    ph = mvs["player_hero"]
    if ph.empty:
        df["star_threat"] = 0.5
        return df

    match_ids = df["match_id"].unique().tolist()
    roster = _load_roster(engine, match_ids)

    if roster.empty:
        df["star_threat"] = 0.5
        return df

    # For each (match_id, opp_team), get the opponent roster.
    roster_uq = roster.drop_duplicates(subset=["match_id", "team_id", "account_id"])

    # Each player's best hero (highest shrunk_wr)
    player_best = ph.loc[
        ph.groupby("account_id")["ph_comfort"].idxmax()
    ].rename(columns={"ph_comfort": "signature_comfort"})[
        ["account_id", "signature_comfort"]
    ]

    # Cross each candidate with the opponent roster.
    cross = df[["match_id", "opp_team", "hero_id"]].drop_duplicates().merge(
        roster_uq.rename(columns={"team_id": "opp_roster_team"}),
        left_on=["match_id", "opp_team"],
        right_on=["match_id", "opp_roster_team"],
        how="inner",
    )

    # Merge each opponent player's signature comfort.
    cross = cross.merge(player_best, on="account_id", how="left")
    cross["signature_comfort"] = cross["signature_comfort"].fillna(0.5)

    # Average across the opponent roster.
    avg_threat = cross.groupby(["match_id", "hero_id"], as_index=False)["signature_comfort"].mean()
    avg_threat.rename(columns={"signature_comfort": "star_threat"}, inplace=True)

    df = df.merge(avg_threat, on=["match_id", "hero_id"], how="left")
    df["star_threat"] = df["star_threat"].fillna(0.5)
    return df


def compute_features(candidates: pd.DataFrame, settings: Settings) -> pd.DataFrame:
    """Compute the 8-feature vector for each candidate row.

    Each candidate row represents a (match, suggested hero) pair.
    Positive samples (label=1.0) are actual picks; negative samples
    (label=0.0) are undrafted heroes.

    Feature order must match FEATURE_SPEC_VERSION in feature_specs.py:
      0. team_picks
      1. team_wr_shrunk
      2. mean_syn_with_allies
      3. mean_counter_vs_enemies
      4. hero_meta_primary_attr
      5. hero_meta_role_count
      6. player_comfort
      7. star_threat

    Missing MV data defaults to 0 (counts) or 0.5 (win rates).
    """
    engine = get_engine(settings)

    # Refresh MVs before reading — they're created WITH NO DATA and
    # stale if the migration hasn't been refreshed recently.
    print("Refreshing materialized views...")
    _refresh_mvs(engine)

    mvs = _load_mvs(engine)

    result = candidates.copy()

    # Drop rows where team IDs are NULL — can't compute features.
    before = len(result)
    result = result.dropna(subset=["acting_team", "opp_team"])
    if len(result) < before:
        print(f"Dropped {before - len(result)} rows with missing team IDs")
    result = _features_team(result, mvs)
    result = _features_synergy(result, mvs)
    result = _features_counter(result, mvs)
    result = _features_hero_meta(result, mvs)
    result = _features_player_comfort(result, mvs, engine)
    result = _features_star_threat(result, mvs, engine)

    return result
