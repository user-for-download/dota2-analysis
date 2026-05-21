-- KEYS[1] = set (ZSET of candidates)
-- KEYS[2] = stats:<url> (HASH)
-- ARGV[1] = url
-- ARGV[2] = success boost

redis.call('HINCRBY',  KEYS[2], 'success', 1)
redis.call('HSET',     KEYS[2], 'consecutive_fail', 0)
redis.call('ZINCRBY',  KEYS[1], tonumber(ARGV[2]), ARGV[1])
return 1