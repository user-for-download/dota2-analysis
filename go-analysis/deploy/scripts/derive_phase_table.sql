-- =====================================================
-- derive_phase_table.sql
-- Extracts the empirical phase table from public.picks_bans
-- for the current (most recent) patch.
--
-- Read-only: SELECT only, no writes to public.*
-- Idempotent: safe to re-run.
-- =====================================================

-- Generate empirical phase table for current patch.
-- Returns one row per draft slot (ord) with:
--   team_mode    — most common team (0=Radiant, 1=Dire) for this slot
--   all_is_pick_same — true if every match agrees on is_pick for this slot
SELECT ord,
       MODE() WITHIN GROUP (ORDER BY team) AS team_mode,
       bool_and(is_pick) AS all_is_pick_same
FROM picks_bans
WHERE match_id IN (
    SELECT match_id FROM matches
    WHERE patch_id = (SELECT MAX(patch_id) FROM matches)
    LIMIT 5000
)
GROUP BY ord
ORDER BY ord;
