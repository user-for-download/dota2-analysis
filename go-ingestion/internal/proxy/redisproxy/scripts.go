package redisproxy

import _ "embed"

//go:embed lua/acquire.lua
var luaAcquire string

//go:embed lua/release.lua
var luaRelease string

//go:embed lua/rate_limit.lua
var luaRateLimit string

//go:embed lua/record_success.lua
var luaRecordSuccess string

//go:embed lua/record_failure.lua
var luaRecordFailure string
