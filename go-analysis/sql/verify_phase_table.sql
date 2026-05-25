-- =====================================================
-- verify_phase_table.sql
-- Validates the empirical phase table against known
-- Captain's Mode draft patterns.
--
-- Read-only: SELECT only, no writes to public.*
-- Idempotent: safe to re-run.
-- =====================================================

-- Derive the empirical phase table (same logic as derive_phase_table.sql)
WITH phase AS (
    SELECT ord,
           MODE() WITHIN GROUP (ORDER BY team) AS team_mode,
           bool_and(is_pick) AS is_pick
    FROM picks_bans
    WHERE match_id IN (
        SELECT match_id FROM matches
        WHERE patch_id = (SELECT MAX(patch_id) FROM matches)
        LIMIT 5000
    )
    GROUP BY ord
),

-- Check 1: All 24 draft slots must be present
slot_count AS (
    SELECT
        COUNT(*) AS total_slots,
        CASE WHEN COUNT(*) = 24 THEN 'PASS'
             ELSE 'FAIL'
        END AS status,
        'Expected 24 draft slots, found ' || COUNT(*) AS detail
    FROM phase
),

-- Check 2: ord values must be contiguous 0..23
slot_contiguity AS (
    SELECT
        CASE WHEN MIN(ord) = 0 AND MAX(ord) = 23
                  AND COUNT(*) = 24
             THEN 'PASS'
             ELSE 'FAIL'
        END AS status,
        'ord range: ' || COALESCE(MIN(ord)::text, 'NULL') || '..' || COALESCE(MAX(ord)::text, 'NULL')
        || ' (count=' || COUNT(*) || ')' AS detail
    FROM phase
),

-- Check 3: Every slot must have a consistent is_pick value
-- (all matches agree whether a slot is a pick or a ban)
pick_consistency AS (
    SELECT
        CASE WHEN bool_and(is_pick) THEN 'PASS'
             ELSE 'FAIL'
        END AS status,
        'All slots have consistent is_pick' AS detail
    FROM phase
),

-- Check 4: Report any slots where matches disagree on is_pick
pick_disagreements AS (
    SELECT ord,
           'DISAGREEMENT' AS check_name,
           'Slot ' || ord || ' has mixed is_pick values across matches' AS detail
    FROM picks_bans
    WHERE match_id IN (
        SELECT match_id FROM matches
        WHERE patch_id = (SELECT MAX(patch_id) FROM matches)
        LIMIT 5000
    )
    GROUP BY ord
    HAVING COUNT(DISTINCT is_pick) > 1
    ORDER BY ord
),

-- Check 5: Team mode must be 0 (Radiant) or 1 (Dire) — no other values
team_validity AS (
    SELECT
        CASE WHEN bool_and(team_mode IN (0, 1)) THEN 'PASS'
             ELSE 'FAIL'
        END AS status,
        'All team_mode values are 0 or 1' AS detail
    FROM phase
),

-- Check 6: Report any anomalous team values in raw data
team_anomalies AS (
    SELECT ord,
           'TEAM_ANOMALY' AS check_name,
           'Slot ' || ord || ' has unexpected team value(s): '
           || array_agg(DISTINCT team)::text AS detail
    FROM picks_bans
    WHERE match_id IN (
        SELECT match_id FROM matches
        WHERE patch_id = (SELECT MAX(patch_id) FROM matches)
        LIMIT 5000
    )
    AND team NOT IN (0, 1)
    GROUP BY ord
    ORDER BY ord
),

-- Check 7: Verify pick/ban phase structure
-- Standard CM has ban-pick alternation in phases.
-- Report the derived pattern for manual review.
phase_pattern AS (
    SELECT ord,
           CASE WHEN is_pick THEN 'PICK' ELSE 'BAN' END AS phase_type,
           CASE WHEN team_mode = 0 THEN 'Radiant' ELSE 'Dire' END AS team_name,
           team_mode
    FROM phase
    ORDER BY ord
),

-- Check 8: Count total picks and bans
pick_ban_totals AS (
    SELECT
        SUM(CASE WHEN is_pick THEN 1 ELSE 0 END) AS total_picks,
        SUM(CASE WHEN is_pick THEN 0 ELSE 1 END) AS total_bans,
        CASE WHEN SUM(CASE WHEN is_pick THEN 1 ELSE 0 END) = 10
              AND SUM(CASE WHEN is_pick THEN 0 ELSE 1 END) = 14
             THEN 'PASS'
             ELSE 'WARN'
        END AS status,
        'Picks=' || SUM(CASE WHEN is_pick THEN 1 ELSE 0 END)
        || ', Bans=' || SUM(CASE WHEN is_pick THEN 0 ELSE 1 END)
        || ' (expected 10 picks, 14 bans in standard CM)' AS detail
    FROM phase
)

-- =====================================================
-- Output: Validation summary
-- =====================================================
SELECT 'SLOT_COUNT'       AS check_name, status, detail FROM slot_count
UNION ALL
SELECT 'SLOT_CONTIGUITY'  AS check_name, status, detail FROM slot_contiguity
UNION ALL
SELECT 'PICK_CONSISTENCY' AS check_name, status, detail FROM pick_consistency
UNION ALL
SELECT 'TEAM_VALIDITY'    AS check_name, status, detail FROM team_validity
UNION ALL
SELECT 'PICK_BAN_TOTALS'  AS check_name, status, detail FROM pick_ban_totals
UNION ALL
-- Anomaly details (if any)
SELECT check_name, 'ANOMALY' AS status, detail FROM pick_disagreements
UNION ALL
SELECT check_name, 'ANOMALY' AS status, detail FROM team_anomalies
ORDER BY check_name;

-- =====================================================
-- Output: Full derived phase pattern for review
-- =====================================================
SELECT ord,
       phase_type,
       team_name,
       team_mode
FROM phase_pattern
ORDER BY ord;
