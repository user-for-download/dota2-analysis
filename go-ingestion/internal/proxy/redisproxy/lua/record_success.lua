-- KEYS[1] = set (ZSET of candidates)
-- KEYS[2] = stats:<url> (HASH)
-- ARGV[1] = url
-- ARGV[2] = success boost

redis.call('HINCRBY',  KEYS[2], 'success', 1)
redis.call('HSET',     KEYS[2], 'consecutive_fail', 0)

-- Only boost score if the proxy is still in the active ZSET.  ZINCRBY auto-creates
-- elements that don't exist, which would re-activate a proxy that was evicted to
-- cooldown — bypassing the cooldown window and creating a zombie proxy loop.
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then
    redis.call('ZINCRBY', KEYS[1], tonumber(ARGV[2]), ARGV[1])
end

return 1