SELECT match_id, start_time
FROM matches
WHERE start_time >= (EXTRACT(EPOCH FROM NOW() - INTERVAL '180 days'))::BIGINT
    AND lobby_type IN (1,2,6)
ORDER BY start_time DESC
LIMIT 10000;
