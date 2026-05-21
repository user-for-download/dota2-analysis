package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Redis     RedisConfig
	Proxy     ProxyConfig
	Queue     QueueConfig
	Postgres  PostgresConfig
	Payload   PayloadConfig
	Dedup     DedupConfig
	Discovery DiscoveryConfig
	Fetcher   FetcherConfig
	Parser    ParserConfig
	Enrich    EnrichConfig
	Telemetry TelemetryConfig
	Migrator  MigratorConfig
}

type RedisConfig struct {
	Addrs           []string
	Password        string
	DB              int
	PoolSize        int
	MinIdleConns    int
	MaxActiveConns  int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ReadOnly        bool
}

type ProxyConfig struct {
	Hold               time.Duration
	MinPoolSize        int
	KeyPrefix          string
	RateLimitPerSec    int
	RateLimitBurst     int
	RateLimitWindow    time.Duration
	RankingInitial     float64
	RankingSuccess     float64
	RankingFailure     float64
	MaxFailures        int
	SeedFile           string
	CanaryURL          string
	RemoteURL          string
	ValidateTimeout    time.Duration
	ValidateParallel   int
	ValidateChunkSize  int
	ValidateMinPublish int
	RefreshInterval    time.Duration
}

type QueueConfig struct {
	Group          string
	Consumer       string
	MaxLen         int64
	DeleteOnAck    bool
	MaxRetries     int
	MaxBackoff     time.Duration
	FetchStream    string
	FetchDLQStream string
	ParseStream    string
	ParseDLQStream string
	AsyncRetry     bool
	AsyncRetryZSet string
}

type PostgresConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type PayloadConfig struct {
	KeyPrefix  string
	DefaultTTL time.Duration
}

type DedupConfig struct {
	KeyPrefix       string
	TTL             time.Duration
	UseBloom        bool
	BloomCapacity   int64
	BloomErrorRate  float64
}

type DiscoveryConfig struct {
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
	HeroStatsURL     string
	HeroStatsInterval time.Duration
}

type FetcherConfig struct {
	UpstreamURL     string
	Batch           int
	Block           time.Duration
	HTTPTimeout     time.Duration
	PayloadTTL      time.Duration
	WaitTimeout     time.Duration
	MaxProxyRetries int
	ProxyBackoff    time.Duration
	AllowDirect     bool
}

type ParserConfig struct {
	Batch                        int
	Block                        time.Duration
	PartitionMaintenanceInterval time.Duration
}

type TelemetryConfig struct {
	Endpoint   string
	SampleRate float64
}

type MigratorConfig struct {
	DSN           string
	MigrationsDir string
}

type EnrichConfig struct {
	BootstrapPrefix      string
	BootstrapTTL         time.Duration
	DotaConstantsBaseURL string
	HTTPTimeout          time.Duration
	Interval             time.Duration
	RunAtStart           bool
	AllowDirect          bool
	WaitTimeout          time.Duration
	MaxProxyRetries      int
	ProxyBackoff         time.Duration
	LocalBootstrapDir    string
	ForceBootstrap       bool
}

func Load(path string) (*Config, error) {
	if path != "" {
		_ = godotenv.Load(path)
	}

	return &Config{
		Redis: RedisConfig{
			Addrs:           getStrs("REDIS_ADDRS", ","),
			Password:        getStr("REDIS_PASSWORD", ""),
			DB:              getInt("REDIS_DB", 0),
			PoolSize:        getInt("REDIS_POOL_SIZE", 100),
			MinIdleConns:    getInt("REDIS_MIN_IDLE_CONNS", 10),
			MaxActiveConns:  getInt("REDIS_MAX_ACTIVE_CONNS", 0),
			ConnMaxLifetime: getDur("REDIS_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getDur("REDIS_CONN_MAX_IDLE_TIME", 10*time.Minute),
			DialTimeout:     getDur("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:     getDur("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout:    getDur("REDIS_WRITE_TIMEOUT", 3*time.Second),
			ReadOnly:        getBool("REDIS_READ_ONLY", false),
		},
		Proxy: ProxyConfig{
			Hold:               getDur("PROXY_HOLD", 30*time.Second),
			MinPoolSize:        getInt("PROXY_MIN_POOL_SIZE", 0),
			KeyPrefix:          getStr("PROXY_KEY_PREFIX", "dota2:proxy"),
			RateLimitPerSec:    getInt("PROXY_RATE_LIMIT_PER_SEC", 0),
			RateLimitBurst:     getInt("PROXY_RATE_LIMIT_BURST", 0),
			RateLimitWindow:    getDur("PROXY_RATE_LIMIT_WINDOW", 1*time.Second),
			RankingInitial:     getFloat("PROXY_RANKING_INITIAL", 100),
			RankingSuccess:     getFloat("PROXY_RANKING_SUCCESS", 1),
			RankingFailure:     getFloat("PROXY_RANKING_FAILURE", 5),
			MaxFailures:        getInt("PROXY_MAX_FAILURES", 5),
			SeedFile:           getStr("PROXY_SEED_FILE", "proxy.txt"),
			CanaryURL:          getStr("PROXY_CANARY_URL", "https://api.ipify.org"),
			RemoteURL:          getStr("PROXY_REMOTE_URL", ""),
			ValidateTimeout:    getDur("PROXY_VALIDATE_TIMEOUT", 10*time.Second),
			ValidateParallel:   getInt("PROXY_VALIDATE_PARALLEL", 50),
			ValidateChunkSize:  getInt("PROXY_VALIDATE_CHUNK_SIZE", 100),
			ValidateMinPublish: getInt("PROXY_VALIDATE_MIN_PUBLISH", 0),
			RefreshInterval:    getDur("PROXY_REFRESH_INTERVAL", 0),
		},
		Queue: QueueConfig{
			Group:          getStr("QUEUE_GROUP", "workers"),
			Consumer:       getStr("QUEUE_CONSUMER", ""),
			MaxLen:         getInt64("QUEUE_MAX_LEN", 10000),
			DeleteOnAck:    getBool("QUEUE_DELETE_ON_ACK", false),
			MaxRetries:     getInt("QUEUE_MAX_RETRIES", 3),
			MaxBackoff:     getDur("QUEUE_MAX_BACKOFF", 30*time.Second),
			FetchStream:    getStr("QUEUE_FETCH_STREAM", "dota2:fetch"),
			FetchDLQStream: getStr("QUEUE_FETCH_DLQ_STREAM", "dota2:fetch:dlq"),
			ParseStream:    getStr("QUEUE_PARSE_STREAM", "dota2:parse"),
			ParseDLQStream: getStr("QUEUE_PARSE_DLQ_STREAM", "dota2:parse:dlq"),
			AsyncRetry:     getBool("QUEUE_ASYNC_RETRY", false),
			AsyncRetryZSet: getStr("QUEUE_ASYNC_RETRY_ZSET", ""),
		},
		Postgres: PostgresConfig{
			DSN:             getStr("POSTGRES_DSN", ""),
			MaxOpenConns:    getInt("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getInt("POSTGRES_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDur("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getDur("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
		},
		Payload: PayloadConfig{
			KeyPrefix:  getStr("PAYLOAD_KEY_PREFIX", "dota2:payload"),
			DefaultTTL: getDur("PAYLOAD_DEFAULT_TTL", 24*time.Hour),
		},
		Dedup: DedupConfig{
			KeyPrefix:      getStr("DEDUP_KEY_PREFIX", "dota2:seen"),
			TTL:            getDur("DEDUP_TTL", 24*time.Hour),
			UseBloom:       getBool("DEDUP_USE_BLOOM", false),
			BloomCapacity:  getInt64("DEDUP_BLOOM_CAPACITY", 10000000),
			BloomErrorRate: getFloat("DEDUP_BLOOM_ERROR_RATE", 0.01),
		},
		Discovery: DiscoveryConfig{
			UpstreamURL:       getStr("DISCOVERY_UPSTREAM_URL", ""),
			WaitTimeout:       getDur("DISCOVERY_WAIT_TIMEOUT", 5*time.Minute),
			HTTPTimeout:       getDur("DISCOVERY_HTTP_TIMEOUT", 30*time.Second),
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
		},
		Fetcher: FetcherConfig{
			UpstreamURL:     getStr("FETCHER_UPSTREAM_URL", ""),
			Batch:           getInt("FETCHER_BATCH", 10),
			Block:           getDur("FETCHER_BLOCK", 2*time.Second),
			HTTPTimeout:     getDur("FETCHER_HTTP_TIMEOUT", 30*time.Second),
			PayloadTTL:      getDur("FETCHER_PAYLOAD_TTL", 72*time.Hour),
			WaitTimeout:     getDur("FETCHER_WAIT_TIMEOUT", 5*time.Minute),
			MaxProxyRetries: getInt("FETCHER_MAX_PROXY_RETRIES", 1),
			ProxyBackoff:    getDur("FETCHER_PROXY_BACKOFF", 250*time.Millisecond),
			AllowDirect:     getBool("FETCHER_ALLOW_DIRECT", false),
		},
		Parser: ParserConfig{
			Batch:                        getInt("PARSER_BATCH", 10),
			Block:                        getDur("PARSER_BLOCK", 2*time.Second),
			PartitionMaintenanceInterval: getDur("PARSER_PARTITION_MAINTENANCE_INTERVAL", 24*time.Hour),
		},
		Enrich: EnrichConfig{
			BootstrapPrefix:      getStr("ENRICH_BOOTSTRAP_PREFIX", "dota2:enrich"),
			BootstrapTTL:         getDur("ENRICH_BOOTSTRAP_TTL", 30*24*time.Hour),
			DotaConstantsBaseURL: getStr("ENRICH_DOTACONSTANTS_BASE_URL", "https://raw.githubusercontent.com/odota/dotaconstants/master/build"),
			HTTPTimeout:          getDur("ENRICH_HTTP_TIMEOUT", 30*time.Second),
			Interval:             getDur("ENRICH_INTERVAL", 24*time.Hour),
			RunAtStart:           getBool("ENRICH_RUN_AT_START", true),
			AllowDirect:          getBool("ENRICH_ALLOW_DIRECT", false),
			WaitTimeout:          getDur("ENRICH_WAIT_TIMEOUT", 5*time.Minute),
			MaxProxyRetries:      getInt("ENRICH_MAX_PROXY_RETRIES", 5),
			ProxyBackoff:         getDur("ENRICH_PROXY_BACKOFF", 500*time.Millisecond),
			LocalBootstrapDir:    getStr("ENRICH_LOCAL_DIR", ""),
			ForceBootstrap:       getBool("ENRICH_FORCE_BOOTSTRAP", false),
		},
		Telemetry: TelemetryConfig{
			Endpoint:   getStr("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			SampleRate: getFloat("OTEL_SAMPLE_RATE", 1.0),
		},
		Migrator: MigratorConfig{
			DSN:           getStr("MIGRATOR_DSN", ""),
			MigrationsDir: getStr("MIGRATOR_MIGRATIONS_DIR", "/migrations"),
		},
	}, nil
}

func getStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getStrs(key, sep string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, sep)
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts
	}
	if key == "REDIS_ADDRS" {
		return []string{"127.0.0.1:6379"}
	}
	return nil
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseBool(v); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := time.ParseDuration(v); err == nil {
			return n
		}
	}
	return def
}
