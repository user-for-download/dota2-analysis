local key = KEYS[1]
local burst = tonumber(ARGV[1])
local windowMs = tonumber(ARGV[2])

if not windowMs or windowMs < 1 then
    windowMs = 1000
end

local n = redis.call('INCR', key)
if n == 1 then
    redis.call('PEXPIRE', key, windowMs)
end
if n > burst then
    return 0
end
return 1