"""Feature engineering on extracted data.

All database queries filter to professional competitive matches:
- leagueid > 0 (matches with an associated league)
- lobby_type IN (1, 2) (practice or tournament lobbies)
"""
import pandas as pd
import numpy as np
from sqlalchemy import text
from trainer.config import Settings
from trainer.db import get_engine


def _refresh_mvs(engine):
    """Refresh all analytics materialized views so data is current.

    Collects errors from all views and continues through failures,
    rather than aborting at the first one. This prevents a transient
    error on one MV from leaving the rest stale.
    """
    mvs = [
        "mv_team_hero_profile",
        "mv_hero_synergy",
        "mv_hero_counter",
        "mv_player_team_history",
        "mv_player_hero_profile",
    ]
    errors = []
    with engine.connect() as conn:
        for mv in mvs:
            print(f"  Refreshing analytics.{mv}...")
            try:
                conn.execute(text(f"REFRESH MATERIALIZED VIEW analytics.{mv}"))
            except Exception as e:
                errors.append(f"{mv}: {e}")
                print(f"    FAILED (continuing): {e}")
        conn.commit()
    if errors:
        print(f"  MV refresh completed with {len(errors)} error(s):")
        for err in errors:
            print(f"    - {err}")


# ── Fallback: compute MV-like stats from base tables (no 6‑month filter) ─────

_FALLBACK_SQL = {
    "team_hero": text("""
        SELECT tm.team_id, pb.hero_id,
               COUNT(*)::int                              AS team_picks,
               ((SUM(CASE WHEN tm.win THEN 1 ELSE 0 END) + 3.0)
                / (COUNT(*) + 6.0))::real                 AS team_wr_shrunk
        FROM public.team_matches tm
        JOIN public.matches m      ON m.match_id  = tm.match_id AND m.start_time = tm.start_time
        JOIN public.picks_bans pb  ON pb.match_id = m.match_id
                                  AND pb.is_pick  = true
                                  AND pb.team     = CASE WHEN tm.is_radiant THEN 0 ELSE 1 END
        JOIN public.heroes h       ON h.id        = pb.hero_id
        WHERE m.leagueid > 0
          AND m.lobby_type IN (1, 2)
          AND m.radiant_team_id IS NOT NULL
          AND m.dire_team_id    IS NOT NULL
          AND pb.hero_id        > 0
        GROUP BY tm.team_id, pb.hero_id
    """),
    "synergy": text("""
        WITH team_picks AS (
            SELECT m.match_id, pb.team, pb.hero_id,
                   CASE WHEN pb.team = 0 THEN m.radiant_win
                        ELSE NOT m.radiant_win END AS won
            FROM public.picks_bans pb
            JOIN public.matches m ON m.match_id = pb.match_id
            WHERE pb.is_pick    = true
              AND m.leagueid    > 0
              AND m.lobby_type IN (1, 2)
              AND m.radiant_team_id IS NOT NULL
              AND m.dire_team_id    IS NOT NULL
              AND m.radiant_win     IS NOT NULL
              AND pb.hero_id        > 0
        )
        SELECT a.hero_id AS hero_a, b.hero_id AS hero_b,
               ((SUM(a.won::int) + 3.0) / (COUNT(*) + 6.0))::real AS shrunk_wr
        FROM team_picks a
        JOIN team_picks b ON a.match_id = b.match_id AND a.team = b.team AND a.hero_id < b.hero_id
        GROUP BY a.hero_id, b.hero_id
    """),
    "counter": text("""
        WITH match_heroes AS (
            SELECT m.match_id, pb.hero_id, pb.team,
                   CASE WHEN pb.team = 0 THEN m.radiant_win
                        ELSE NOT m.radiant_win END AS won
            FROM public.picks_bans pb
            JOIN public.matches m ON m.match_id = pb.match_id
            WHERE pb.is_pick    = true
              AND m.leagueid    > 0
              AND m.lobby_type IN (1, 2)
              AND m.radiant_team_id IS NOT NULL
              AND m.dire_team_id    IS NOT NULL
              AND m.radiant_win     IS NOT NULL
              AND pb.hero_id        > 0
        )
        SELECT a.hero_id AS hero_a, b.hero_id AS hero_b,
               ((SUM(a.won::int) + 3.0) / (COUNT(*) + 6.0))::real AS shrunk_wr
        FROM match_heroes a
        JOIN match_heroes b ON a.match_id = b.match_id AND a.team != b.team AND a.hero_id != b.hero_id
        GROUP BY a.hero_id, b.hero_id
    """),
    # Player-level hero comfort includes ranked games (solo/team) in addition
    # to professional matches. Team-level guards (leagueid, team_id) are not
    # needed — a pro player's comfort on a hero is meaningful regardless of
    # lobby type. Ranked: 5=ranked_team_mm, 6=ranked_solo_mm, 7=ranked.
    "player_hero": text("""
        SELECT pm.account_id, pm.hero_id,
               ((SUM(pm.win::int) + 5.0) / (COUNT(*) + 10.0))::real AS ph_comfort
        FROM public.player_matches pm
        JOIN public.matches m ON m.match_id = pm.match_id AND m.start_time = pm.start_time
        WHERE m.lobby_type IN (1, 2, 5, 6, 7)
          AND pm.account_id IS NOT NULL
          AND pm.win        IS NOT NULL
          AND pm.hero_id    > 0
        GROUP BY pm.account_id, pm.hero_id
    """),
}


def _compute_hero_global_stats(engine) -> pd.DataFrame:
    """Compute per-hero stats from picks_bans (always available).

    Used as a fallback when team-level MVs are empty — gives the model
    real signal (hero popularity, global win rate) instead of flat 0.5.
    """
    return pd.read_sql(
        text("""
            SELECT
                pb.hero_id,
                COUNT(*)::int
                    AS global_picks,
                ((SUM(CASE WHEN (pb.team = 0 AND m.radiant_win)
                                OR (pb.team = 1 AND NOT m.radiant_win)
                           THEN 1 ELSE 0 END) + 3.0)
                 / (COUNT(*) + 6.0))::real
                    AS global_wr_shrunk
            FROM public.picks_bans pb
            JOIN public.matches m ON m.match_id = pb.match_id
            WHERE pb.is_pick  = true
              AND m.leagueid  > 0
              AND m.lobby_type IN (1, 2)
              AND m.radiant_win IS NOT NULL
              AND pb.hero_id   > 0
            GROUP BY pb.hero_id
        """),
        engine,
    )


def _load_mvs(engine) -> dict[str, pd.DataFrame]:
    """Load all materialized views and reference tables into DataFrames.

    If a MV is empty (training data is older than the 6‑month window),
    falls back to a direct query against base tables with no time filter.
    """
    # ── MVs that may be empty for historical data ──────────────────────
    mv_queries = {
        "team_hero": text("""
            SELECT team_id, hero_id,
                   games::int AS team_picks,
                   shrunk_wr   AS team_wr_shrunk
            FROM analytics.mv_team_hero_profile
        """),
        "synergy": text("SELECT hero_a, hero_b, shrunk_wr FROM analytics.mv_hero_synergy"),
        "counter": text("SELECT hero_a, hero_b, shrunk_wr FROM analytics.mv_hero_counter"),
        "player_hero": text("""
            SELECT account_id, hero_id,
                   shrunk_wr AS ph_comfort
            FROM analytics.mv_player_hero_profile
        """),
    }

    loaded = {}
    for key, sql in mv_queries.items():
        df = pd.read_sql(sql, engine)
        loaded[key] = df

    # ── hero_meta never needs a fallback (static table) ────────────────
    loaded["hero_meta"] = pd.read_sql(
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

    # ── Fall back to base-table queries for empty MVs ──────────────────
    for key in ("team_hero", "synergy", "counter", "player_hero"):
        if loaded[key].empty:
            print(f"  {key} MV is empty — computing from base tables (no time filter)...")
            fallback_df = pd.read_sql(_FALLBACK_SQL[key], engine)
            if not fallback_df.empty:
                loaded[key] = fallback_df
                print(f"    -> loaded {len(fallback_df)} rows from base tables")
            else:
                print(f"    -> base tables also empty, features will use defaults")

    # ── Hero-global stats: always available, used as fillna fallback ───
    loaded["hero_global"] = _compute_hero_global_stats(engine)
    if loaded["hero_global"].empty:
        print("  WARNING: hero_global stats are empty — no picks_bans data found")

    return loaded


def _features_team(
    df: pd.DataFrame, mvs: dict[str, pd.DataFrame]
) -> pd.DataFrame:
    """Features 0-1: team_picks, team_wr_shrunk.

    Falls back to hero-global stats when team-level data is missing
    (empty team_matches or unlinked team IDs).
    """
    hg = mvs.get("hero_global")
    th = mvs["team_hero"]
    df = df.merge(
        th[["team_id", "hero_id", "team_picks", "team_wr_shrunk"]],
        left_on=["acting_team", "hero_id"],
        right_on=["team_id", "hero_id"],
        how="left",
    )
    if hg is not None and not hg.empty:
        # Merge global stats and use them as fallback.
        df = df.merge(
            hg[["hero_id", "global_picks", "global_wr_shrunk"]],
            on="hero_id",
            how="left",
        )
        df["team_picks"] = df["team_picks"].fillna(df["global_picks"]).astype(int)
        df["team_wr_shrunk"] = df["team_wr_shrunk"].fillna(df["global_wr_shrunk"])
        df.drop(columns=["global_picks", "global_wr_shrunk"], inplace=True, errors="ignore")
    else:
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
    hg = mvs.get("hero_global")

    if syn.empty:
        if hg is not None and not hg.empty:
            # Fallback: compute average global WR of ally picks per candidate.
            # ⚠️ Must filter to only allies picked BEFORE this slot — including
            # future picks leaks draft decisions into training features.
            actual = df[df["label"] == 1.0][["match_id", "hero_id", "slot"]].drop_duplicates()
            actual.rename(columns={"hero_id": "ally_hero_id", "slot": "ally_slot"}, inplace=True)
            cross = df[["match_id", "hero_id", "slot"]].drop_duplicates().merge(
                actual, on="match_id"
            )
            cross = cross[cross["ally_slot"] < cross["slot"]]
            cross.drop(columns=["slot", "ally_slot"], inplace=True)
            cross = cross[cross["hero_id"] != cross["ally_hero_id"]]
            cross = cross.merge(
                hg[["hero_id", "global_wr_shrunk"]].rename(
                    columns={"hero_id": "ally_hero_id", "global_wr_shrunk": "syn_fb"}
                ),
                on="ally_hero_id",
                how="left",
            )
            mean_syn = cross.groupby(["match_id", "hero_id"], as_index=False)["syn_fb"].mean()
            mean_syn.rename(columns={"syn_fb": "mean_syn_with_allies"}, inplace=True)
            df = df.merge(mean_syn, on=["match_id", "hero_id"], how="left")
            df["mean_syn_with_allies"] = df["mean_syn_with_allies"].fillna(0.5)
        else:
            df["mean_syn_with_allies"] = 0.5
        return df

    # Actual picks (positive samples) define the ally set.
    # ⚠️ Include slot on both sides and filter so allies picked AT or AFTER
    # the current candidate's slot are excluded — they're future draft decisions
    # that should NOT be visible to the model.
    actual = df[df["label"] == 1.0][
        ["match_id", "hero_id", "acting_team", "slot"]
    ].drop_duplicates()
    if actual.empty:
        df["mean_syn_with_allies"] = 0.5
        return df

    ally_slot_cols = ["match_id", "ally_hero_id", "acting_team", "ally_slot"]

    # Cross each candidate with actual picks on the SAME team before this slot.
    cross = df[["match_id", "hero_id", "acting_team", "slot"]].drop_duplicates().merge(
        actual.rename(columns={"hero_id": "ally_hero_id", "slot": "ally_slot"})[ally_slot_cols],
        on=["match_id", "acting_team"],
    )
    cross = cross[cross["ally_slot"] < cross["slot"]]  # ← future-pick guard
    cross.drop(columns=["slot", "ally_slot"], inplace=True)
    cross = cross[cross["hero_id"] != cross["ally_hero_id"]]

    # Normalise pair order for synergy lookup.
    cross["a"] = cross[["hero_id", "ally_hero_id"]].min(axis=1)
    cross["b"] = cross[["hero_id", "ally_hero_id"]].max(axis=1)
    cross = cross.merge(syn, left_on=["a", "b"], right_on=["hero_a", "hero_b"], how="left")

    # If synergy is missing for some pairs, fill with hero_global WR.
    if hg is not None and not hg.empty:
        cross = cross.merge(
            hg[["hero_id", "global_wr_shrunk"]].rename(
                columns={"hero_id": "ally_hero_id", "global_wr_shrunk": "global_wr_ally"}
            ),
            on="ally_hero_id",
            how="left",
        )
        cross["shrunk_wr"] = cross["shrunk_wr"].fillna(cross["global_wr_ally"])
        cross.drop(columns=["global_wr_ally"], inplace=True, errors="ignore")

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
    hg = mvs.get("hero_global")

    if cnt.empty:
        if hg is not None and not hg.empty:
            # Fallback: compute average global WR of enemy picks per candidate.
            # ⚠️ Include slot and filter to enemy picks BEFORE this slot only.
            actual = df[df["label"] == 1.0][
                ["match_id", "hero_id", "acting_team", "opp_team", "slot"]
            ].drop_duplicates()
            enemy_picks = actual.rename(
                columns={"hero_id": "enemy_hero_id", "acting_team": "enemy_acting", "slot": "enemy_slot"}
            )[["match_id", "enemy_hero_id", "enemy_acting", "enemy_slot"]]
            cross = df[["match_id", "hero_id", "opp_team", "slot"]].drop_duplicates()
            cross = cross.merge(
                enemy_picks,
                left_on=["match_id", "opp_team"],
                right_on=["match_id", "enemy_acting"],
                how="inner",
            )
            cross = cross[cross["enemy_slot"] < cross["slot"]]  # ← future-pick guard
            cross.drop(columns=["slot", "enemy_slot"], inplace=True)
            cross = cross.merge(
                hg[["hero_id", "global_wr_shrunk"]].rename(
                    columns={"hero_id": "enemy_hero_id", "global_wr_shrunk": "cnt_fb"}
                ),
                on="enemy_hero_id",
                how="left",
            )
            mean_cnt = cross.groupby(["match_id", "hero_id"], as_index=False)["cnt_fb"].mean()
            mean_cnt.rename(columns={"cnt_fb": "mean_counter_vs_enemies"}, inplace=True)
            df = df.merge(mean_cnt, on=["match_id", "hero_id"], how="left")
            df["mean_counter_vs_enemies"] = df["mean_counter_vs_enemies"].fillna(0.5)
        else:
            df["mean_counter_vs_enemies"] = 0.5
        return df

    # Actual picks with their team perspective.
    actual = df[df["label"] == 1.0][
        ["match_id", "hero_id", "acting_team", "opp_team", "slot"]
    ].drop_duplicates()
    if actual.empty:
        df["mean_counter_vs_enemies"] = 0.5
        return df

    # For each actual pick, the *enemy* picks are the rows in the same
    # match where acting_team = this row's opp_team.
    enemy_map = actual.rename(
        columns={"hero_id": "enemy_hero_id", "acting_team": "enemy_acting",
                 "opp_team": "enemy_opp", "slot": "enemy_slot"}
    )[["match_id", "enemy_hero_id", "enemy_acting", "enemy_slot"]]

    # Cross each candidate with enemy picks in the same match.
    cross = df[["match_id", "hero_id", "acting_team", "opp_team", "slot"]].drop_duplicates()

    # Find enemy picks: rows in same match where acting_team = candidate's opp_team
    # We join cross with enemy_map: candidate's opp_team matches enemy's acting_team
    # Ensure enemy_map has no duplicate join keys — duplicates would cause
    # a silent Cartesian explosion in the inner merge below.
    enemy_dupes = enemy_map.duplicated(subset=["match_id", "enemy_acting"])
    if enemy_dupes.any():
        print(f"WARNING: {enemy_dupes.sum()} duplicate enemy entries before merge")
    enemy_map = enemy_map[~enemy_dupes]

    cross = cross.merge(
        enemy_map,
        left_on=["match_id", "opp_team"],
        right_on=["match_id", "enemy_acting"],
        how="inner",
    )
    cross = cross[cross["enemy_slot"] < cross["slot"]]  # ← future-pick guard
    cross.drop(columns=["slot", "enemy_slot"], inplace=True)

    # Counter: hero_a = candidate, hero_b = enemy.
    cross = cross.merge(
        cnt,
        left_on=["hero_id", "enemy_hero_id"],
        right_on=["hero_a", "hero_b"],
        how="left",
    )

    # Fill missing counter pairs with enemy's global WR.
    if hg is not None and not hg.empty:
        cross = cross.merge(
            hg[["hero_id", "global_wr_shrunk"]].rename(
                columns={"hero_id": "enemy_hero_id", "global_wr_shrunk": "global_wr_enemy"}
            ),
            on="enemy_hero_id",
            how="left",
        )
        cross["shrunk_wr"] = cross["shrunk_wr"].fillna(cross["global_wr_enemy"])
        cross.drop(columns=["global_wr_enemy"], inplace=True, errors="ignore")

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
            JOIN public.matches m ON m.match_id = pm.match_id
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
    hg = mvs.get("hero_global")

    if ph.empty:
        if hg is not None and not hg.empty:
            # Fallback: global WR per hero (no player-specific signal).
            df = df.merge(
                hg[["hero_id", "global_wr_shrunk"]],
                on="hero_id",
                how="left",
            )
            df["player_comfort"] = df["global_wr_shrunk"].fillna(0.5)
            df.drop(columns=["global_wr_shrunk"], inplace=True, errors="ignore")
        else:
            df["player_comfort"] = 0.5
        return df

    match_ids = df["match_id"].unique().tolist()
    roster = _load_roster(engine, match_ids)

    if roster.empty:
        if hg is not None and not hg.empty:
            df = df.merge(
                hg[["hero_id", "global_wr_shrunk"]],
                on="hero_id",
                how="left",
            )
            df["player_comfort"] = df["global_wr_shrunk"].fillna(0.5)
            df.drop(columns=["global_wr_shrunk"], inplace=True, errors="ignore")
        else:
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
    if hg is not None and not hg.empty:
        cross = cross.merge(
            hg[["hero_id", "global_wr_shrunk"]],
            on="hero_id",
            how="left",
        )
        cross["ph_comfort"] = cross["ph_comfort"].fillna(cross["global_wr_shrunk"])
        cross.drop(columns=["global_wr_shrunk"], inplace=True, errors="ignore")
    else:
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
    hg = mvs.get("hero_global")

    if ph.empty:
        if hg is not None and not hg.empty:
            # Fallback: average global WR across all heroes as generic threat.
            df["star_threat"] = hg["global_wr_shrunk"].mean()
        else:
            df["star_threat"] = 0.5
        return df

    match_ids = df["match_id"].unique().tolist()
    roster = _load_roster(engine, match_ids)

    if roster.empty:
        if hg is not None and not hg.empty:
            df["star_threat"] = hg["global_wr_shrunk"].mean()
        else:
            df["star_threat"] = 0.5
        return df

    # For each (match_id, opp_team), get the opponent roster.
    # Assert dedup is already done at the SQL layer — if not, catch regressions.
    dupes = roster.duplicated(subset=["match_id", "team_id", "account_id"])
    if dupes.any():
        print(f"WARNING: {dupes.sum()} duplicate roster rows found — SQL query needs DISTINCT")
    roster_uq = roster[~dupes]  # safe dedup (but push to SQL in production)

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


# ── Per-candidate features (vary across heroes within a decision group) ──
#
# Critical insight for LambdaMART: a feature must deliver RANKING SIGNAL,
# which requires it to TAKE DIFFERENT VALUES for different candidates
# inside the same decision.  Features that are constant across all 31
# candidates (e.g. team-level stats when MVs are empty) contribute zero
# ranking information and waste the model's capacity.
#
# The functions below compute features that genuinely vary per candidate
# hero, giving LambdaMART something to learn from.


def _features_hero_priors(
    candidates: pd.DataFrame, raw_decisions: pd.DataFrame
) -> pd.DataFrame:
    """Features 8-10: hero_pick_rate, hero_wr, hero_popularity.

    ITEM-LEVEL PRIORS computed from the full training corpus.  Each hero's
    global pick frequency and win rate varies per candidate, which is the
    minimum requirement for ranking signal.

    Not label leakage: with 31 candidates per decision, knowing "Snapfire is
    generally popular" does not reveal "Snapfire was the chosen pick in this
    specific draft context".  The model needs other features (composition,
    counters) to make that determination — same as how search ranking uses
    "document length" without spoiling the relevance label.

    Edge cases:
    - Heroes with zero picks get shrink-fitted defaults (pick_rate ~2/N, wr=0.5).
    - Missing heroes in the merge get the same defaults via fillna.
    """
	picks = raw_decisions[raw_decisions["is_pick"] == True]

	hero_stats = (
		picks.groupby("hero_id")
		.agg(
			hero_pick_count=("match_id", "count"),
			hero_win_count=("acting_won", "sum"),
		)
		.reset_index()
	)

	# Shrunk pick rate: beta prior with strength=2.
	# Denominator = total_picks (sum across all heroes), NOT n_decisions (unique
	# matches).  n_decisions × 10 ≈ total_picks, but the former gives rates > 1.0
	# for commonly-picked heroes (e.g. 2000 picks / 2000 matches ≈ 1.0), which
	# breaks the model's split thresholds.  The Go inference side must use the
	# same denominator — see HeroPickRateSource in hero_prior_source.go.
	total_picks = hero_stats["hero_pick_count"].sum()
	hero_stats["hero_pick_rate"] = (
		(hero_stats["hero_pick_count"] + 2.0) / (total_picks + 4.0)
	)

    # Shrunk win rate: beta prior with strength=10 (moderate regularization).
    hero_stats["hero_wr"] = (
        (hero_stats["hero_win_count"] + 10.0)
        / (hero_stats["hero_pick_count"] + 20.0)
    )

    # Log-scaled pick count captures long-tail popularity where the raw
    # count dominates the pick-rate / n_decisions denominator.
    hero_stats["hero_popularity"] = np.log1p(hero_stats["hero_pick_count"])

    result = candidates.merge(
        hero_stats[["hero_id", "hero_pick_rate", "hero_wr", "hero_popularity"]],
        on="hero_id",
        how="left",
    )
    # Fill heroes not present in training data (edge case for candidate gen).
    result["hero_pick_rate"] = result["hero_pick_rate"].fillna(2.0 / (n_decisions + 4.0))
    result["hero_wr"] = result["hero_wr"].fillna(0.5)
    result["hero_popularity"] = result["hero_popularity"].fillna(0.0)
    return result


def _features_attr_diversity(candidates: pd.DataFrame) -> pd.DataFrame:
    """Features 11-14: attr_is_str, attr_is_agi, attr_is_int, attr_fit_score.

    Attribute-based draft-context features.  These vary per candidate because
    each hero has a DIFFERENT primary attribute.  The model can learn
    composition heuristics:

      "Our team already has 2 STR cores → pick an INT support (attr_is_int=1)"
      "No AGI heroes on our team → Templar Assassin fills the gap (attr_is_agi=1)"

    attr_fit_score = team_picks × attr_weight
      - INT heroes get 0.5 weight (burst vs STR/AGI)
      - AGI heroes get 0.3 weight (DPS complement)
      - STR heroes get 0.2 weight (frontline)

    The multiplication by team_picks makes the signal stronger in the late
    draft (composition matters more with 4 picks decided than 1).

    Requires hero_meta_primary_attr to already exist in candidates (must run
    AFTER _features_hero_meta).
    """
    result = candidates.copy()

    # One-hot from categorical primary_attr (1=str, 2=agi, 3=int, 4=all).
    result["attr_is_str"] = (result["hero_meta_primary_attr"] == 1).astype(float)
    result["attr_is_agi"] = (result["hero_meta_primary_attr"] == 2).astype(float)
    result["attr_is_int"] = (result["hero_meta_primary_attr"] == 3).astype(float)

    # 'all' heroes (primary_attr=4) get all three flags — they truly flex.
    result["attr_is_str"] = np.where(
        result["hero_meta_primary_attr"] == 4, 1.0, result["attr_is_str"]
    )
    result["attr_is_agi"] = np.where(
        result["hero_meta_primary_attr"] == 4, 1.0, result["attr_is_agi"]
    )
    result["attr_is_int"] = np.where(
        result["hero_meta_primary_attr"] == 4, 1.0, result["attr_is_int"]
    )

    # Attribute-fit score: hero's attribute value amplified by draft depth.
    # team_picks_before (0-4) is the number of picks the team has made in this
    # draft before the current slot — it's the same for all candidates in a
    # decision, but the attribute flags differ, so the product varies per hero.
    #
    # Weights: INT=0.5 (burst vs STR/AGI), AGI=0.3 (DPS complement),
    # STR=0.2 (frontline).  Universal heroes (all three flags=1) use the
    # MAX weight (0.5) via np.maximum, not the SUM (1.0), preventing 3x bias.
    result["attr_fit_score"] = (
        result["team_picks_before"]
        * np.maximum(
            result["attr_is_int"] * 0.5,
            np.maximum(
                result["attr_is_agi"] * 0.3,
                result["attr_is_str"] * 0.2,
            ),
        )
    )

    return result


def _features_draft(candidates: pd.DataFrame, raw_decisions: pd.DataFrame | None = None) -> pd.DataFrame:
    """Features 15-16+: draft_slot_norm, is_pick_phase, semantic draft context.

    Draft position and pick-vs-ban phase — no DB needed, no label leakage.
    When raw_decisions is provided, also computes semantic draft features
    that survive format changes (team_picks_made, is_first_pick, etc.).
    """
    result = candidates.copy()
    result["draft_slot_norm"] = result["slot"] / 30.0  # normalize to ~[0, 1]
    result["is_pick_phase"] = result["is_pick"].astype(float)
    result["is_pick_phase"] = result["is_pick_phase"].fillna(1.0)

    if raw_decisions is not None:
        result = _features_draft_semantic(result, raw_decisions)

    return result


def _features_draft_semantic(candidates: pd.DataFrame, raw_decisions: pd.DataFrame) -> pd.DataFrame:
    """Features 17-23: semantic draft context that survives format changes.

    Instead of absolute slot position (which shifts when Valve changes the
    draft format), compute relative state: how many picks each team has made
    at this point in the draft.  Feature values are the same for all candidates
    in a slot — they provide temporal context, not per-hero ranking signal.

    All counts represent the state BEFORE the current slot (decisions already
    made, not including the current one).
    """
    # Compute cumulative draft state from raw decisions.
    dec = raw_decisions.sort_values(["match_id", "slot"]).copy()
    dec["is_pick_int"] = dec["is_pick"].astype(int)
    dec["is_ban_int"] = (~dec["is_pick"]).astype(int)

    # Per-team running counts (picks/bans BEFORE this slot).
    grp_team = dec.groupby(["match_id", "team"])
    dec["team_picks_before"] = (
        grp_team["is_pick_int"].cumsum() - dec["is_pick_int"]
    ).astype(int)
    dec["team_bans_before"] = (
        grp_team["is_ban_int"].cumsum() - dec["is_ban_int"]
    ).astype(int)

    # Totals across both teams.
    grp_match = dec.groupby("match_id")
    dec["total_picks_before"] = (
        grp_match["is_pick_int"].cumsum() - dec["is_pick_int"]
    ).astype(int)
    dec["total_bans_before"] = (
        grp_match["is_ban_int"].cumsum() - dec["is_ban_int"]
    ).astype(int)

    # Enemy team stats = total - own team's.
    dec["enemy_picks_before"] = (dec["total_picks_before"] - dec["team_picks_before"]).astype(int)

    # Per-slot state for merging back to candidates.
    slot_state = dec[
        ["match_id", "slot", "team_picks_before", "enemy_picks_before"]
    ].drop_duplicates()

    result = candidates.merge(slot_state, on=["match_id", "slot"], how="left")

    # Fill missing values (should not occur with real data).
    result["team_picks_before"] = result["team_picks_before"].fillna(0).astype(int)
    result["enemy_picks_before"] = result["enemy_picks_before"].fillna(0).astype(int)

    # Derived boolean / relative features.
    # 5 picks per team is the CM default — this can be parameterised once
    # the draft schema registry (Tier 2) is implemented.
    result["is_first_pick"] = (result["team_picks_before"] == 0).astype(float)
    result["is_last_pick"] = (result["team_picks_before"] == 4).astype(float)
    result["is_counter_phase"] = (
        result["enemy_picks_before"] > result["team_picks_before"]
    ).astype(float)
    result["remaining_team_picks"] = (5 - result["team_picks_before"]).clip(0, 5).astype(float)
    result["draft_progress"] = (
        (result["team_picks_before"] + result["enemy_picks_before"]) / 10.0
    ).clip(0, 1)  # 10 total picks in CM

    return result


def compute_features(
    candidates: pd.DataFrame, settings: Settings, raw_decisions: pd.DataFrame
) -> pd.DataFrame:
    """Compute the feature vector for each candidate row.

    Each candidate row represents a (match, suggested hero) pair.
    Positive samples (label=1.0) are actual picks; negative samples
    (label=0.0) are undrafted heroes.

    Critical: for LambdaMART ranking, a feature must VARY across
    candidates within the same decision group.  If every candidate
    gets the same value (e.g. team-level defaults from empty MVs),
    the feature contributes zero ranking signal.

    Features 0-7   MV-dependent — may return constants when tables empty.
    Features 8-10  Hero priors — VARY per candidate by hero_id.
    Features 11-14 Attribute/draft-context — VARY per candidate by attr.
    Features 15-16 Draft position — same across all candidates in group,
                   included as weak group-level signal.
    Features 17-23 Semantic draft context — patch-invariant relative state
                   (team_picks_before, is_first_pick, draft_progress, etc.).

    Feature order must match FEATURE_SPEC_VERSION in feature_specs.py.
    """
    engine = get_engine(settings)

    # Refresh MVs before reading — they're created WITH NO DATA and
    # stale if the migration hasn't been refreshed recently.
    print("Refreshing materialized views...")
    _refresh_mvs(engine)

    mvs = _load_mvs(engine)

    # Warn about empty MVs — features relying on them will default to 0.5/0.
    for key, df in mvs.items():
        if df.empty:
            print(f"  WARNING: analytics.{key} is empty — some features will use defaults")

    result = candidates.copy()

    # Impute missing team IDs with sentinel instead of dropping rows.
    before = len(result)
    result["acting_team"] = result["acting_team"].fillna(-1).astype(int)
    result["opp_team"] = result["opp_team"].fillna(-1).astype(int)
    if len(result) < before:
        print(f"Imputed team IDs for {before - len(result)} rows (should not happen with fillna)")

    # MV-dependent features (may use defaults when MVs empty).
    # These are mostly constant across candidates unless MVs are populated.
    result = _features_team(result, mvs)
    result = _features_synergy(result, mvs)
    result = _features_counter(result, mvs)
    result = _features_hero_meta(result, mvs)    # adds hero_meta_primary_attr
    result = _features_player_comfort(result, mvs, engine)
    result = _features_star_threat(result, mvs, engine)

    # ── Per-candidate features (VARY across heroes — real ranking signal) ──
    # hero_priors: pick-rate, wr, popularity — computed from raw decisions.
    result = _features_hero_priors(result, raw_decisions)

    # draft context: position + semantic features (same per group, weak signal).
    # Must run BEFORE _features_attr_diversity so that team_picks_before is
    # available as a column (computed inside _features_draft_semantic).
    result = _features_draft(result, raw_decisions=raw_decisions)

    # attr_diversity: attribute flags + fit score.
    # Must run AFTER _features_hero_meta (needs hero_meta_primary_attr).
    result = _features_attr_diversity(result)

    return result
