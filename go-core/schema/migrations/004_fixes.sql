-- =====================================================
-- 004_fixes.sql — Post-launch schema hardening
--
-- Prerequisites:
--   - docker-compose must use a pg_cron-enabled image
--     (ghcr.io/citusdata/pg_cron:pg16) and set
--     `shared_preload_libraries=pg_cron` via command args.
--   - Existing 001-003 migrations must have been applied.
-- =====================================================

-- =====================================================
-- pg_cron Extension & MV Refresh Schedule
-- Requires: pg_cron in shared_preload_libraries
-- =====================================================
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- =====================================================
-- Helper Functions for Locked MV Refreshes
-- Standalone functions so that cron.schedule() only needs
-- to call a simple SELECT (nested DO blocks inside the
-- schedule string are not valid SQL).
-- =====================================================

CREATE OR REPLACE FUNCTION analytics.refresh_mv_team_hero_profile_locked()
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    -- Use xact-level lock (pg_try_advisory_xact_lock) so it's auto-released
    -- on abort if REFRESH MATERIALIZED VIEW fails. Session-level locks
    -- (pg_try_advisory_lock) are never released on function abort, causing
    -- permanent freeze of all future cron executions.
    IF pg_try_advisory_xact_lock(hashtext('r_mv_team_hero_profile')) THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_team_hero_profile;
    ELSE
        RAISE NOTICE 'Skipping mv_team_hero_profile: lock held';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION analytics.refresh_mv_hero_synergy_locked()
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    IF pg_try_advisory_xact_lock(hashtext('r_mv_hero_synergy')) THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_hero_synergy;
    ELSE
        RAISE NOTICE 'Skipping mv_hero_synergy: lock held';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION analytics.refresh_mv_hero_counter_locked()
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    IF pg_try_advisory_xact_lock(hashtext('r_mv_hero_counter')) THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_hero_counter;
    ELSE
        RAISE NOTICE 'Skipping mv_hero_counter: lock held';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION analytics.refresh_mv_player_team_history_locked()
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    IF pg_try_advisory_xact_lock(hashtext('r_mv_player_team_history')) THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_player_team_history;
    ELSE
        RAISE NOTICE 'Skipping mv_player_team_history: lock held';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION analytics.refresh_mv_player_hero_profile_locked()
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    IF pg_try_advisory_xact_lock(hashtext('r_mv_player_hero_profile')) THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_player_hero_profile;
    ELSE
        RAISE NOTICE 'Skipping mv_player_hero_profile: lock held';
    END IF;
END;
$$;

-- =====================================================
-- Schedule Jobs via cron.schedule()
-- Each calls a simple SELECT function — no nested DO blocks.
-- Overlaps can be monitored via:
--   SELECT * FROM cron.job_run_details ORDER BY start_time DESC LIMIT 10;
-- =====================================================
DO $cron$
BEGIN
    PERFORM cron.schedule(
        'refresh-mv-team-hero-profile', '0 2 * * *',
        $cmd$SELECT analytics.refresh_mv_team_hero_profile_locked()$cmd$
    );
    PERFORM cron.schedule(
        'refresh-mv-hero-synergy', '0 2 * * *',
        $cmd$SELECT analytics.refresh_mv_hero_synergy_locked()$cmd$
    );
    PERFORM cron.schedule(
        'refresh-mv-hero-counter', '0 2 * * *',
        $cmd$SELECT analytics.refresh_mv_hero_counter_locked()$cmd$
    );
    PERFORM cron.schedule(
        'refresh-mv-player-team-history', '0 2 * * *',
        $cmd$SELECT analytics.refresh_mv_player_team_history_locked()$cmd$
    );
    PERFORM cron.schedule(
        'refresh-mv-player-hero-profile', '0 2 * * *',
        $cmd$SELECT analytics.refresh_mv_player_hero_profile_locked()$cmd$
    );
    PERFORM cron.schedule(
        'maintain-time-partitions', '0 1 * * 0',
        $cmd$SELECT ensure_future_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], 8)$cmd$
    );
    PERFORM cron.schedule(
        'drop-old-partitions', '0 3 * * 0',
        $cmd$SELECT drop_old_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], 2)$cmd$
    );
EXCEPTION WHEN OTHERS THEN
    RAISE WARNING 'pg_cron scheduling skipped: % (is shared_preload_libraries=pg_cron set?)', SQLERRM;
END;
$cron$;

-- =====================================================
-- Partition Retention
-- Drop time-range partitions older than retention_years.
-- Naming convention: {parent_table}_{year}_q{quarter}
-- Safe: skips non-matching names, uses DROP TABLE IF EXISTS.
-- =====================================================
CREATE OR REPLACE FUNCTION drop_old_time_partitions(
    parents         TEXT[],
    retention_years INT DEFAULT 2
)
RETURNS TABLE(parent_table TEXT, partition_name TEXT, status TEXT)
LANGUAGE plpgsql
AS $$
DECLARE
    parent      TEXT;
    part        RECORD;
    part_year   INT;
    cutoff_year INT;
BEGIN
    cutoff_year := EXTRACT(YEAR FROM NOW())::INT - retention_years;

    FOREACH parent IN ARRAY parents LOOP
        FOR part IN
            SELECT ch.relname              AS name,
                   ch.relnamespace::regnamespace::text AS schema_name
            FROM pg_inherits  i
            JOIN pg_class     ch ON ch.oid = i.inhrelid
            JOIN pg_class     pt ON pt.oid = i.inhparent
            JOIN pg_namespace n  ON n.oid  = pt.relnamespace
            WHERE pt.relname = parent
              AND n.nspname  = current_schema()
              AND ch.relname ~ ('^' || parent || '_\d{4}_q[1-4]$')
        LOOP
            -- Extract year: {parent}_YYYY_q{Q}
            part_year := substring(part.name FROM '_(\d{4})_q')::INT;

            IF part_year < cutoff_year THEN
                BEGIN
                    EXECUTE format('DROP TABLE IF EXISTS %I.%I', part.schema_name, part.name);
                    parent_table   := parent;
                    partition_name := part.name;
                    status         := 'DROPPED';
                    RETURN NEXT;
                EXCEPTION WHEN OTHERS THEN
                    parent_table   := parent;
                    partition_name := part.name;
                    status         := 'ERROR: ' || SQLERRM;
                    RETURN NEXT;
                END;
            END IF;
        END LOOP;
    END LOOP;

    -- Always return at least one row if nothing was dropped
    IF NOT FOUND THEN
        parent_table   := '';
        partition_name := '';
        status         := 'NO_PARTITIONS_TO_DROP';
        RETURN NEXT;
    END IF;
END;
$$;

COMMENT ON FUNCTION drop_old_time_partitions(TEXT[], INT) IS
    'Drop quarterly time-range partitions older than retention_years. '
    'Safe for idempotent / scheduled calls. '
    'Usage: SELECT * FROM drop_old_time_partitions(ARRAY[''matches'',''player_matches'',''public_matches'',''player_timeseries''], 2);';

-- =====================================================
-- Stub Hero Monitoring
-- Queries v_unknown_heroes and returns entries older than
-- the threshold (default 1 hour).
-- =====================================================
CREATE OR REPLACE FUNCTION check_stale_stubs(
    max_age INTERVAL DEFAULT INTERVAL '1 hour'
)
RETURNS TABLE(hero_id INT, name TEXT, age INTERVAL)
LANGUAGE plpgsql STABLE
AS $$
BEGIN
    RETURN QUERY
    SELECT h.id, h.name, h.age::INTERVAL
    FROM v_unknown_heroes h
    WHERE h.age > max_age
    ORDER BY h.age DESC;

    IF NOT FOUND THEN
        RETURN QUERY
        SELECT 0::INT, 'ALL_STUBS_CURRENT'::TEXT, '0'::INTERVAL;
    END IF;
END;
$$;

COMMENT ON FUNCTION check_stale_stubs(INTERVAL) IS
    'Return stub-hero entries older than max_age (default 1h). '
    'Usage: SELECT * FROM check_stale_stubs(); '
    'Alert if hero_id != 0.';

-- =====================================================
-- Composite Indexes for Analytics Query Performance
-- =====================================================

-- Account × hero × patch covering index.
-- Used by mv_player_team_history and mv_player_hero_profile
-- which JOIN matches ON player_matches(account_id, hero_id, patch_id).
CREATE INDEX IF NOT EXISTS idx_pm_account_hero_patch
    ON player_matches(account_id, hero_id, patch_id, start_time DESC)
    WHERE account_id IS NOT NULL AND patch_id IS NOT NULL;

-- Common match join on both team IDs (used by team-level MVs).
CREATE INDEX IF NOT EXISTS idx_matches_radiant_dire_win
    ON matches(radiant_team_id, dire_team_id, start_time DESC)
    INCLUDE (radiant_win)
    WHERE radiant_team_id IS NOT NULL AND dire_team_id IS NOT NULL;

-- =====================================================
-- Hash Partition Indexing Note
--
-- Hash-partitioned tables (player_match_details(32), match_advantages(8),
-- match_cosmetics(8), picks_bans(16), draft_timings(16)) rely on the
-- primary-key index on match_id that PostgreSQL creates automatically on
-- every partition. Point lookups by match_id are efficient.
--
-- No secondary local indexes are added because the current query patterns
-- against these tables are exclusively match_id lookups. If new analytics
-- queries need to filter on other columns (e.g., hero_id across all
-- partitions), add local indexes on the parent table and PostgreSQL will
-- propagate them to each partition automatically.
-- =====================================================

-- =====================================================
-- Record Migration
-- =====================================================
INSERT INTO public.schema_migrations (version, filename)
VALUES (4, '004_fixes.sql')
ON CONFLICT (version) DO NOTHING;
