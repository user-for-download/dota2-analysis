local availableKey = KEYS[1]
local leasedKey = KEYS[2]
local leaseKey = KEYS[3]
local ttl = tonumber(ARGV[1])
local token = ARGV[2]
local topN = tonumber(ARGV[3]) or 20
-- ARGV[4] = random seed from Go (deterministic replication)
local now = tonumber(ARGV[5]) -- unix timestamp from Go (deterministic)
math.randomseed(tonumber(ARGV[4]) or now)

-- Clean up expired leases
redis.call('ZREMRANGEBYSCORE', leasedKey, 0, now - 1)

-- Scan only the top (topN * 2) candidates by score instead of the entire ZSET.
-- ZREVRANGE with 0,-1 on a pool of 5000+ proxies allocates a giant Lua table on
-- every acquisition, blocking the single-threaded Redis event loop.
local windowSize = topN * 2
local candidates = redis.call('ZREVRANGE', availableKey, 0, windowSize - 1)
if #candidates == 0 then
	return nil
end

-- Collect only unleased candidates from the window.
local available = {}
for _, c in ipairs(candidates) do
	if not redis.call('ZSCORE', leasedKey, c) then
		table.insert(available, c)
		if #available == topN then
			break
		end
	end
end

if #available == 0 then
	return nil
end

-- Pick randomly from available proxies to distribute load
local pick = available[math.random(1, #available)]
local expiresAt = now + ttl

redis.call('ZADD', leasedKey, expiresAt, pick)
redis.call('SET', leaseKey, pick, 'EX', ttl)
return pick