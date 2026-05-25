local availableKey = KEYS[1]
local leasedKey = KEYS[2]
local leaseKey = KEYS[3]
local ttl = tonumber(ARGV[1])
local token = ARGV[2]
local topN = tonumber(ARGV[3]) or 20
local now = tonumber(redis.call('TIME')[1])
-- ARGV[4] is a random seed from Go ensuring deterministic replication
-- (redis.call('TIME') is non-deterministic and would break replica sync)
math.randomseed(tonumber(ARGV[4]) or now)

-- Clean up expired leases
redis.call('ZREMRANGEBYSCORE', leasedKey, 0, now - 1)

local candidates = redis.call('ZREVRANGE', availableKey, 0, topN - 1)
if #candidates == 0 then
    return nil
end

-- Collect only unleased candidates
local available = {}
for _, c in ipairs(candidates) do
    if not redis.call('ZSCORE', leasedKey, c) then
        table.insert(available, c)
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