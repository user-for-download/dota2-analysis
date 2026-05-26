-- MV Population Diagnostic
-- Run against the database to understand why analytics MVs are empty.
-- Usage: psql $POSTGRES_DSN -f diagnose_mvs.sql

WITH counts AS (
    SELECT 'team_matches'             AS tbl, COUNT(*) AS rows FROM public.team_matches
    UNION ALL
    SELECT 'matches (league)'        , COUNT(*) FROM public.matches WHERE leagueid > 0
    UNION ALL
    SELECT 'matches (teams not null)', COUNT(*) FROM public.matches
        WHERE leagueid > 0 AND radiant_team_id IS NOT NULL AND dire_team_id IS NOT NULL
    UNION ALL
    SELECT 'picks_bans'              , COUNT(*) FROM public.picks_bans
    UNION ALL
    SELECT 'player_matches'          , COUNT(*) FROM public.player_matches
    UNION ALL
    SELECT 'heroes'                  , COUNT(*) FROM public.heroes
    UNION ALL
    SELECT 'matches (last 6mo)'      , COUNT(*) FROM public.matches
        WHERE leagueid > 0
        AND start_time >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
    UNION ALL
    SELECT 'matches (all time)'      , COUNT(*) FROM public.matches
        WHERE leagueid > 0
)
SELECT * FROM counts ORDER BY tbl;

-- Time range of available matches
SELECT
    to_timestamp(MIN(start_time))  AS earliest_match,
    to_timestamp(MAX(start_time))  AS latest_match,
    COUNT(*)                       AS total_pro_matches,
    COUNT(*) FILTER (
        WHERE start_time >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
    )                              AS within_6mo_window
FROM public.matches
WHERE leagueid > 0;

-- Sample team_matches rows (if any exist)
SELECT * FROM public.team_matches LIMIT 5;

-- Does the picks_bans join with team_matches work?
SELECT COUNT(*) AS joinable_picks
FROM public.picks_bans pb
JOIN public.matches m ON m.match_id = pb.match_id
JOIN public.team_matches tm
    ON tm.match_id = pb.match_id
    AND tm.team_id = CASE WHEN pb.team = 0 THEN m.radiant_team_id ELSE m.dire_team_id END
WHERE pb.is_pick = true
  AND m.leagueid > 0;
