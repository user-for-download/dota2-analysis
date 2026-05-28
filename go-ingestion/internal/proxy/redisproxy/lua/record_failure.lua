-- KEYS[1] = set (ZSET of candidates)
-- KEYS[2] = stats:<url> (HASH)
-- KEYS[3] = cooldown ZSET
-- KEYS[4] = cooldown entry TTL key
-- ARGV[1] = url
-- ARGV[2] = failure penalty (float)
-- ARGV[3] = max consecutive failures (int, 0 = disabled)
-- ARGV[4] = last error string
-- ARGV[5] = cooldown seconds (int, default 300 = 5 minutes)
-- ARGV[6] = now (unix timestamp from Go, for deterministic replication)

local url       = ARGV[1]
local penalty   = tonumber(ARGV[2])
local maxFails  = tonumber(ARGV[3])
local lastErr   = ARGV[4]
local coolSecs  = tonumber(ARGV[5]) or 300
local now       = tonumber(ARGV[6])

redis.call('HINCRBY',  KEYS[2], 'fail', 1)
local streak = redis.call('HINCRBY', KEYS[2], 'consecutive_fail', 1)
redis.call('HSET',     KEYS[2], 'last_error', lastErr)

if maxFails > 0 and streak >= maxFails then
    redis.call('ZREM', KEYS[1], url)
    redis.call('ZADD', KEYS[3], now + coolSecs, url)
    return 1
end

redis.call('ZINCRBY', KEYS[1], -penalty, url)
return 0