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

-- Helper: scan a slice of the sorted set by rank and collect unleased proxies.
-- Returns a table of available proxy strings (up to topN) and the rank just
-- past the last checked entry (for pagination), or nil if no more entries.
local function scanWindow(start_, limit_)
	local candidates = redis.call('ZREVRANGE', availableKey, start_, start_ + limit_ - 1)
	if #candidates == 0 then
		return nil, start_
	end
	local avail = {}
	for _, c in ipairs(candidates) do
		if not redis.call('ZSCORE', leasedKey, c) then
			table.insert(avail, c)
			if #avail == topN then
				break
			end
		end
	end
	return avail, start_ + limit_
end

-- Scan the first window (top proxies by score).  Under normal load this
-- small window is fast and sufficient; under high concurrency the fallback
-- prevents artificial "pool exhausted" errors when the top N proxies happen
-- to all be leased simultaneously.
local windowSize = topN * 2
local available, nextStart = scanWindow(0, windowSize)

-- Fallback: if the window had candidates but all were leased, scan deeper
-- slices in windowSize chunks (up to 10 windows = ~400 entries at default
-- topN=20).  If the window returned nil the ZSET is empty — bail immediately.
if available ~= nil and #available == 0 then
	local maxWindows = 10
	local w = 1
	while (available ~= nil and #available == 0) and w < maxWindows do
		available, nextStart = scanWindow(nextStart, windowSize)
		w = w + 1
	end
end

if available == nil or #available == 0 then
	return nil
end

-- Pick randomly from available proxies to distribute load
local pick = available[math.random(1, #available)]
local expiresAt = now + ttl

redis.call('ZADD', leasedKey, expiresAt, pick)
redis.call('SET', leaseKey, pick, 'EX', ttl)
return pick