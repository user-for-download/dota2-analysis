-- =====================================================
-- 002_analytics.sql — Analytics schema for draft analysis
-- =====================================================
-- Read-side only. All derived data lives in analytics.*.
-- Never writes to public.*.
--
-- Objects:
--   Schema:        analytics
--   Materialized Views:
--     mv_team_hero_profile    — team-level hero pick/win rates (shrunk WR, prior=3)
--     mv_hero_synergy         — hero pair synergy on same team (shrunk WR, prior=3)
--     mv_hero_counter         — hero-vs-hero counter matchups (shrunk WR, prior=3)
--     mv_player_team_history  — player-team aggregated recent games
--     mv_player_hero_profile  — player-level hero comfort (shrunk WR, prior=5)
--   Tables:
--     feature_snapshots_player_hero — point-in-time player-hero snapshots
--     featurizer_runs               — single-row featurizer tracking
--   Roles:
--     analytics_reader — SELECT on public.* and analytics.*
--     analytics_writer — full access to analytics.*, SELECT on public.*
-- =====================================================

-- ----- Schema --------------------------------------------------------
CREATE SCHEMA IF NOT EXISTS analytics;

COMMENT ON SCHEMA analytics IS
    'Derived analytics data for draft analysis. Read-side only — no writes to public.*';

-- ----- Drop existing objects (idempotent re-run) ---------------------
-- WARNING: CASCADE drops any views, functions, or other objects that
-- depend on these MVs. Ensure no downstream objects reference them
-- directly, or re-create those objects after migration.
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_player_hero_profile    CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_player_team_history    CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_hero_counter           CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_hero_synergy           CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_team_hero_profile      CASCADE;

-- =====================================================================
-- 1. mv_team_hero_profile
--    Team-level hero pick/win rates with shrunk win rate (prior=3).
--    One row per (team_id, hero_id).
-- =====================================================================
CREATE MATERIALIZED VIEW analytics.mv_team_hero_profile AS
SELECT
    tm.team_id,
    pb.hero_id,
    h.localized_name  AS hero_name,
    COUNT(*)          AS games,
    SUM(tm.win::int)  AS wins,
    ((SUM(tm.win::int) + 3.0) / (COUNT(*) + 6.0))::real AS shrunk_wr,
    NOW()             AS refreshed_at
FROM public.team_matches tm
JOIN public.matches m
    ON  m.match_id   = tm.match_id
    AND m.start_time = tm.start_time
JOIN public.picks_bans pb
    ON  pb.match_id = m.match_id
    AND pb.is_pick  = true
    AND pb.team     = CASE WHEN tm.is_radiant THEN 0 ELSE 1 END
JOIN public.heroes h ON h.id = pb.hero_id
WHERE m.leagueid          > 0
  AND m.radiant_team_id   IS NOT NULL
  AND m.dire_team_id      IS NOT NULL
  AND m.start_time        >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
  AND pb.hero_id          > 0          -- exclude no_hero stub
GROUP BY tm.team_id, pb.hero_id, h.localized_name
WITH NO DATA;

CREATE UNIQUE INDEX uq_mv_team_hero_profile
    ON analytics.mv_team_hero_profile (team_id, hero_id);

-- =====================================================================
-- 2. mv_hero_synergy
--    Hero-pair synergy: how often two heroes win together on the same
--    team. Shrunk WR with prior=3. One row per unordered pair
--    (hero_a < hero_b).
-- =====================================================================
CREATE MATERIALIZED VIEW analytics.mv_hero_synergy AS
WITH team_picks AS (
    SELECT
        m.match_id,
        pb.team,
        pb.hero_id,
        CASE WHEN pb.team = 0 THEN m.radiant_win
             ELSE NOT m.radiant_win
        END AS won
    FROM public.picks_bans pb
    JOIN public.matches m ON m.match_id = pb.match_id
    WHERE pb.is_pick        = true
      AND m.leagueid        > 0
      AND m.radiant_team_id IS NOT NULL
      AND m.dire_team_id    IS NOT NULL
      AND m.radiant_win     IS NOT NULL
      AND m.start_time      >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
      AND pb.hero_id        > 0
)
SELECT
    a.hero_id                                       AS hero_a,
    ha.localized_name                               AS hero_a_name,
    b.hero_id                                       AS hero_b,
    hb.localized_name                               AS hero_b_name,
    COUNT(*)                                        AS games,
    SUM(a.won::int)                                 AS wins,
    ((SUM(a.won::int) + 3.0) / (COUNT(*) + 6.0))::real AS shrunk_wr,
    NOW()                                           AS refreshed_at
FROM team_picks a
JOIN team_picks b
    ON  a.match_id  = b.match_id
    AND a.team      = b.team
    AND a.hero_id   < b.hero_id
JOIN public.heroes ha ON ha.id = a.hero_id
JOIN public.heroes hb ON hb.id = b.hero_id
GROUP BY a.hero_id, ha.localized_name,
         b.hero_id, hb.localized_name
WITH NO DATA;

CREATE UNIQUE INDEX uq_mv_hero_synergy
    ON analytics.mv_hero_synergy (hero_a, hero_b);

-- =====================================================================
-- 3. mv_hero_counter
--    Hero-vs-hero counter matchups: hero_a's win rate when facing
--    hero_b on the opposing team. Shrunk WR with prior=3.
--    Both directions stored: (A,B) and (B,A) are separate rows.
-- =====================================================================
CREATE MATERIALIZED VIEW analytics.mv_hero_counter AS
WITH match_heroes AS (
    SELECT
        m.match_id,
        pb.hero_id,
        pb.team,
        CASE WHEN pb.team = 0 THEN m.radiant_win
             ELSE NOT m.radiant_win
        END AS won
    FROM public.picks_bans pb
    JOIN public.matches m ON m.match_id = pb.match_id
    WHERE pb.is_pick        = true
      AND m.leagueid        > 0
      AND m.radiant_team_id IS NOT NULL
      AND m.dire_team_id    IS NOT NULL
      AND m.radiant_win     IS NOT NULL
      AND m.start_time      >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
      AND pb.hero_id        > 0
)
SELECT
    a.hero_id                                       AS hero_a,
    ha.localized_name                               AS hero_a_name,
    b.hero_id                                       AS hero_b,
    hb.localized_name                               AS hero_b_name,
    COUNT(*)                                        AS games,
    SUM(a.won::int)                                 AS wins,
    ((SUM(a.won::int) + 3.0) / (COUNT(*) + 6.0))::real AS shrunk_wr,
    NOW()                                           AS refreshed_at
FROM match_heroes a
JOIN match_heroes b
    ON  a.match_id  = b.match_id
    AND a.team      != b.team
    AND a.hero_id   != b.hero_id
JOIN public.heroes ha ON ha.id = a.hero_id
JOIN public.heroes hb ON hb.id = b.hero_id
GROUP BY a.hero_id, ha.localized_name,
         b.hero_id, hb.localized_name
WITH NO DATA;

CREATE UNIQUE INDEX uq_mv_hero_counter
    ON analytics.mv_hero_counter (hero_a, hero_b);

-- =====================================================================
-- 4. mv_player_team_history
--    Player-team aggregated recent games within the patch window.
--    Identifies which team a player has been playing for.
-- =====================================================================
CREATE MATERIALIZED VIEW analytics.mv_player_team_history AS
SELECT
    pm.account_id,
    tm.team_id,
    COUNT(*)              AS games,
    SUM(tm.win::int)      AS wins,
    MAX(tm.start_time)    AS last_match_time,
    MAX(m.patch_id)       AS last_patch_id,
    NOW()                 AS refreshed_at
FROM public.player_matches pm
JOIN public.team_matches tm
    ON  tm.match_id   = pm.match_id
    AND tm.start_time = pm.start_time
    AND tm.is_radiant = pm.is_radiant
JOIN public.matches m
    ON  m.match_id   = pm.match_id
    AND m.start_time = pm.start_time
WHERE m.leagueid          > 0
  AND m.radiant_team_id   IS NOT NULL
  AND m.dire_team_id      IS NOT NULL
  AND m.start_time        >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
  AND pm.account_id       IS NOT NULL
GROUP BY pm.account_id, tm.team_id
WITH NO DATA;

CREATE UNIQUE INDEX uq_mv_player_team_history
    ON analytics.mv_player_team_history (account_id, team_id);

-- =====================================================================
-- 5. mv_player_hero_profile
--    Player-level hero comfort: win rate per (player, hero) with
--    shrunk WR (prior=5 — higher prior for noisier player data).
-- =====================================================================
CREATE MATERIALIZED VIEW analytics.mv_player_hero_profile AS
SELECT
    pm.account_id,
    pm.hero_id,
    h.localized_name                              AS hero_name,
    COUNT(*)                                      AS games,
    SUM(pm.win::int)                              AS wins,
    ((SUM(pm.win::int) + 5.0) / (COUNT(*) + 10.0))::real AS shrunk_wr,
    NOW()                                         AS refreshed_at
FROM public.player_matches pm
JOIN public.matches m
    ON  m.match_id   = pm.match_id
    AND m.start_time = pm.start_time
JOIN public.heroes h ON h.id = pm.hero_id
WHERE m.leagueid          > 0
  AND m.radiant_team_id   IS NOT NULL
  AND m.dire_team_id      IS NOT NULL
  AND m.start_time        >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
  AND pm.account_id       IS NOT NULL
  AND pm.win              IS NOT NULL
  AND pm.hero_id          > 0
GROUP BY pm.account_id, pm.hero_id, h.localized_name
WITH NO DATA;

CREATE UNIQUE INDEX uq_mv_player_hero_profile
    ON analytics.mv_player_hero_profile (account_id, hero_id);

-- ----- Populate MVs with existing data -------------------------------
-- Standard REFRESH (not CONCURRENTLY) is safe here because:
--   - The migration holds exclusive schema lock
--   - No application reads are active during migration
--   - On first run the MVs are empty (no-op); on re-runs this re-pouplates
REFRESH MATERIALIZED VIEW analytics.mv_team_hero_profile;
REFRESH MATERIALIZED VIEW analytics.mv_hero_synergy;
REFRESH MATERIALIZED VIEW analytics.mv_hero_counter;
REFRESH MATERIALIZED VIEW analytics.mv_player_team_history;
REFRESH MATERIALIZED VIEW analytics.mv_player_hero_profile;

-- =====================================================================
-- 6. feature_snapshots_player_hero
--    Point-in-time snapshots of player-hero features. Populated by
--    the featurizer when it refreshes MVs. Enables tracking how a
--    player's comfort with a hero evolves over time.
-- =====================================================================
CREATE TABLE IF NOT EXISTS analytics.feature_snapshots_player_hero (
    snapshot_id    BIGINT GENERATED ALWAYS AS IDENTITY,
    snapshot_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    account_id     BIGINT      NOT NULL,
    hero_id        SMALLINT    NOT NULL,
    games          INTEGER     NOT NULL,
    wins           INTEGER     NOT NULL,
    shrunk_wr      REAL        NOT NULL,
    patch_min      INTEGER,
    patch_max      INTEGER,
    PRIMARY KEY (snapshot_id, account_id, hero_id)
);

CREATE INDEX idx_fsph_account_hero
    ON analytics.feature_snapshots_player_hero (account_id, hero_id, snapshot_at DESC);
CREATE INDEX idx_fsph_snapshot_at
    ON analytics.feature_snapshots_player_hero (snapshot_at DESC);

COMMENT ON TABLE analytics.feature_snapshots_player_hero IS
    'Point-in-time snapshots of mv_player_hero_profile. INSERTed by featurizer on each refresh cycle.';

-- =====================================================================
-- 7. featurizer_runs
--    Single-row table tracking the featurizer's last execution.
--    The CHECK constraint enforces exactly one row.
-- =====================================================================
CREATE TABLE IF NOT EXISTS analytics.featurizer_runs (
    id                 INTEGER PRIMARY KEY DEFAULT 1,
    last_mv_refresh_at TIMESTAMPTZ,
    last_snapshot_at   TIMESTAMPTZ,
    mv_refresh_status  TEXT,
    snapshot_status    TEXT,
    patch_min          INTEGER,
    patch_max          INTEGER,
    mv_rows_total      BIGINT,
    snapshot_rows      BIGINT,
    duration_ms        INTEGER,
    error_message      TEXT,
    CONSTRAINT single_row CHECK (id = 1)
);

-- Seed the single row
INSERT INTO analytics.featurizer_runs (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE analytics.featurizer_runs IS
    'Single-row featurizer tracking. Updated by cmd/featurizer after each refresh cycle.';

-- =====================================================================
-- 8. Roles and permissions
--    analytics_reader — read-only access to public.* and analytics.*
--    analytics_writer — full access to analytics.*, read-only public.*
-- =====================================================================

-- Reader role
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'analytics_reader') THEN
        CREATE ROLE analytics_reader;
    END IF;
END $$;

GRANT USAGE ON SCHEMA public    TO analytics_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public    TO analytics_reader;
GRANT USAGE ON SCHEMA analytics TO analytics_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO analytics_reader;

-- Writer role
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'analytics_writer') THEN
        CREATE ROLE analytics_writer;
    END IF;
END $$;

GRANT USAGE ON SCHEMA analytics TO analytics_writer;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA analytics TO analytics_writer;
GRANT USAGE ON SCHEMA public    TO analytics_writer;
GRANT SELECT ON ALL TABLES IN SCHEMA public    TO analytics_writer;

-- Default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO analytics_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT SELECT ON TABLES TO analytics_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT ALL PRIVILEGES ON TABLES TO analytics_writer;

-- =====================================================================
-- Record migration
-- =====================================================================
INSERT INTO public.schema_migrations (version, filename)
VALUES (2, '002_analytics.sql')
ON CONFLICT (version) DO NOTHING;
