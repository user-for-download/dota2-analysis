package discovery

import (
	"os"
	"strconv"
	"time"
)

// Config holds discovery configuration loaded from DISCOVERY_* environment variables.
type Config struct {
	UpstreamURL       string
	WaitTimeout       time.Duration
	HTTPTimeout       time.Duration
	Interval          time.Duration
	RunAtStart        bool
	AllowDirect       bool
	MaxRetries        int
	RetryBackoff      time.Duration
	MinProxyPoolSize  int
	QueriesDir        string
	DefaultQueryKey   string
	LeagueQueriesDir  string
	LeagueInterval    time.Duration
	TeamQueriesDir    string
	TeamInterval      time.Duration
	ProPlayerURL      string
	ProPlayerInterval time.Duration
	HeroStatsURL      string
	HeroStatsInterval time.Duration
}

// LoadConfig reads DISCOVERY_* environment variables and returns a Config with
// defaults matching go-ingestion/internal/config/config.go DiscoveryConfig.
func LoadConfig() Config {
	return Config{
		UpstreamURL:       getStr("DISCOVERY_UPSTREAM_URL", ""),
		WaitTimeout:       getDur("DISCOVERY_WAIT_TIMEOUT", 5*time.Minute),
		HTTPTimeout:       getDur("DISCOVERY_HTTP_TIMEOUT", 180*time.Second),
		Interval:          getDur("DISCOVERY_INTERVAL", 24*time.Hour),
		RunAtStart:        getBool("DISCOVERY_RUN_AT_START", true),
		AllowDirect:       getBool("DISCOVERY_ALLOW_DIRECT", false),
		MaxRetries:        getInt("DISCOVERY_MAX_RETRIES", 8),
		RetryBackoff:      getDur("DISCOVERY_RETRY_BACKOFF", 500*time.Millisecond),
		MinProxyPoolSize:  getInt("DISCOVERY_MIN_PROXY_POOL_SIZE", 1),
		QueriesDir:        getStr("DISCOVERY_QUERIES_DIR", "/queries"),
		DefaultQueryKey:   getStr("DISCOVERY_DEFAULT_KEY", "default"),
		LeagueQueriesDir:  getStr("DISCOVERY_LEAGUE_QUERIES_DIR", ""),
		LeagueInterval:    getDur("DISCOVERY_LEAGUE_INTERVAL", 6*time.Hour),
		TeamQueriesDir:    getStr("DISCOVERY_TEAM_QUERIES_DIR", ""),
		TeamInterval:      getDur("DISCOVERY_TEAM_INTERVAL", 6*time.Hour),
		ProPlayerURL:      getStr("DISCOVERY_PRO_PLAYER_URL", "https://api.opendota.com/api/proPlayers"),
		ProPlayerInterval: getDur("DISCOVERY_PRO_PLAYER_INTERVAL", 6*time.Hour),
		HeroStatsURL:      getStr("DISCOVERY_HERO_STATS_URL", "https://api.opendota.com/api/heroStats"),
		HeroStatsInterval: getDur("DISCOVERY_HERO_STATS_INTERVAL", 6*time.Hour),
	}
}

func getStr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

func getBool(k string, d bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return d
}

func getDur(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if t, err := time.ParseDuration(v); err == nil {
			return t
		}
	}
	return d
}
