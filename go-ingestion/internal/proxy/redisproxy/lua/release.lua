local leasedKey = KEYS[1]
local leaseKey = KEYS[2]

local data = redis.call('GET', leaseKey)
if not data then
    return 0
end

redis.call('ZREM', leasedKey, data)
redis.call('DEL', leaseKey)
return 1