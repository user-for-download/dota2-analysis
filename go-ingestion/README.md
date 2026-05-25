# go-ingestion

A distributed pipeline for discovering, fetching, parsing, and enriching
Dota 2 match data. Written in Go, backed by Redis (queues, proxy pool,
caches) and Postgres (durable storage).

## Features

- **Staged microservices** — discoverer → fetcher → parser → ingester,
  plus an out-of-band enricher for static reference data
- **Smart proxy pool** — ranked, leased, rate-limited; HTTP / HTTPS /
  SOCKS5 supported; atomic operations via embedded Redis Lua scripts
- **Resilient queues** — Redis Streams with consumer groups, retry
  with backoff + jitter, and dead-letter streams
- **Idempotent ingestion** — payload TTLs + dedup set + DB uniqueness
- **Pluggable storage** — ports/adapters layout; in-memory adapters
  ship for tests
- **Observability** — OpenTelemetry (traces + metrics) via OTLP,
  W3C trace context propagation through Redis Streams,
 Jaeger UI at http://localhost:16686

For the design rationale and component-level details, see
[ARCHITECTURE.md](ARCHITECTURE.md).

## Pipeline at a Glance

```
proxyloader ─▶ proxy pool (Redis)
                      │
                      ▼ leased by fetcher
discoverer ─▶ fetch queue ─▶ fetcher ─▶ parse queue ─▶ parser ─▶ Postgres
                        │              ▲
                        └──── payload blobs ────┘
                        (Redis, TTL)

DLQ      ─▶ dead-letter inspection (fetcher/parse failures)

enricher ─▶ Postgres (heroes, items, patches, …)
migrator ─▶ Postgres schema
Jaeger  ─▶ http://localhost:16686 (traces + metrics)
```

## Quick Start

### Prerequisites

- Docker + Docker Compose with the Buildx plugin
- `make`
- A list of working proxies in `go-ingestion/config/proxy.txt` (see
  `go-ingestion/config/proxy.txt.example`)
- Fetch external reference data: `make bootstrap` or `make fetch-constants && make fetch-proxies`
  (these download dotaconstants JSON and proxy lists before first run)

### One-command bring-up

```sh
# Build all images and run the full stack in the foreground
make up
```

To run detached:

```sh
make upd
```

The first invocation also runs database migrations. Once the proxy
pool reaches `PROXY_MIN_POOL_SIZE`, the discoverer starts pushing
match IDs and the rest of the pipeline picks up.

### Useful targets

```sh
make help             # list available targets
make build            # build all images (cached)
make rebuild          # build without cache
make up-db            # start only Redis + Postgres
make migrate          # run migrations once
make fetch-constants  # download dotaconstants JSON to go-ingestion/assets/dotaconstants/
make fetch-proxies    # download proxy list to go-ingestion/config/proxy.txt
make bootstrap        # fetch-constants + fetch-proxies (run before first startup)
make shell-db         # open psql on the running container
make shell-redis      # open redis-cli
make downv            # stop everything and remove volumes
make armageddon       # nuke project images, volumes, build cache
```

## Repository Layout

```
go-ingestion/               ← This module
  cmd/                      Entry points (one main per binary)
    discoverer/  fetcher/  parser/  enricher/
    proxyloader/  migrator/  dlq/
  internal/
    bootstrap/              Wires Redis/Postgres/metrics from Config
    config/                 Env-driven configuration with all settings
    dedup/  payload/  queue/ Domain abstractions + adapters
    proxy/                  Pool, loader, validator, transports
    metrics/                Sink, in-mem/otelmetrics variants
    enrich/                 Reference-data sources (RunSource interface)
    enrich/sources/         dotaconstants providers
    storage/                matchstore, refdatastore, lookupstore, postgres, redis, …
    worker/                 Pipeline runner + Handler interface
    proxy/httpdo/           OTel-wrapped HTTPDoer
    queue/redisstreams/     Queue with W3C trace propagation
  assets/
    queries/                Discoverer SQL queries (one .sql per key, one subdir per cycle)
    dotaconstants/          Cached dotaconstants JSON for offline enricher bootstrap
  build/
    dockerfiles/            One Dockerfile per binary, sharing a base
  config/
    proxy.txt               Proxy seed file
    proxy.txt.example       Proxy seed format
  Makefile                  Module build targets

deploy/                     ← Shared orchestration (repo root)
  docker-bake.hcl           Buildx bake definition
  docker-compose.yml        Service composition (profiles: db/migrate/workers/all)
  .env / .env.example       Consolidated environment variables (all 149 vars)
```

## Configuration

All settings come from environment variables. The canonical source of
truth is `deploy/.env` (149 vars, consolidated at the repo root) —
copy it to `deploy/.env` and edit. See also
`internal/config/config.go` for Go-side defaults.
The most commonly overridden ones:

### Redis

| Variable                  | Default          |
|---------------------------|------------------|
| `REDIS_ADDRS`             | `127.0.0.1:6379` |
| `REDIS_PASSWORD`          |                  |
| `REDIS_DB`                | `0`              |
| `REDIS_POOL_SIZE`         | `100`            |

### Postgres

| Variable                  | Default |
|---------------------------|---------|
| `POSTGRES_DSN`            | *(required)* |
| `POSTGRES_MAX_OPEN_CONNS` | `25`    |
| `POSTGRES_MAX_IDLE_CONNS` | `5`     |

### Discovery

| Variable                   | Default                       |
|----------------------------|-------------------------------|
| `DISCOVERY_UPSTREAM_URL`   | *(required)* explorer endpoint |
| `DISCOVERY_QUERIES_DIR`    | `/queries`                    |
| `DISCOVERY_DEFAULT_KEY`   | `default`                     |
| `DISCOVERY_INTERVAL`      | `24h`                         |
| `DISCOVERY_RUN_AT_START`  | `true`                        |

### Fetcher

| Variable                   | Default |
|----------------------------|---------|
| `FETCHER_UPSTREAM_URL`     | *(required)* — match ID appended by code (no `%d`!) |
| `FETCHER_BATCH`            | `10`    |
| `FETCHER_HTTP_TIMEOUT`     | `30s`   |
| `FETCHER_PAYLOAD_TTL`      | `72h`   |
| `FETCHER_MAX_PROXY_RETRIES`| `5`     |

### Proxy pool

| Variable                  | Default               |
|---------------------------|-----------------------|
| `PROXY_SEED_FILE`         | `proxy.txt`           |
| `PROXY_REMOTE_URL`        | *(optional)*          |
| `PROXY_MIN_POOL_SIZE`     | `0` (no wait)         |
| `PROXY_HOLD`              | `30s`                 |
| `PROXY_VALIDATE_PARALLEL`  | `50`                  |
| `PROXY_VALIDATE_CHUNK_SIZE`| `100`               |
| `PROXY_MAX_FAILURES`     | `5` (then evict)      |
| `PROXY_REFRESH_INTERVAL`  | `0` (one-shot)        |
| `PROXY_FORCE_REFRESH_INTERVAL` | `1h` (unconditional reload) |

### Queue

| Variable                  | Default          |
|---------------------------|------------------|
| `QUEUE_GROUP`             | `workers`        |
| `QUEUE_MAX_LEN`           | `10000`          |
| `QUEUE_MAX_RETRIES`       | `3`              |
| `QUEUE_MAX_BACKOFF`       | `30s`            |

### Parser

| Variable                                | Default | Description |
|-----------------------------------------|---------|-------------|
| `PARSER_BATCH`                          | `10`    | Number of tasks to pop per iteration |
| `PARSER_BLOCK`                          | `2s`    | How long to block waiting for tasks |
| `PARSER_PARTITION_MAINTENANCE_INTERVAL` | `24h`   | How often to ensure future quarterly partitions exist |

### Enrichment

| Variable                    | Default                                                              |
|----------------------------|----------------------------------------------------------------------|
| `ENRICH_DOTACONSTANTS_BASE_URL` | `https://raw.githubusercontent.com/odota/dotaconstants/master/build` |
| `ENRICH_LOCAL_DIR`       | *(empty)*                                                       | Path to local dotaconstants JSON for offline bootstrap (e.g. `/dotaconstants`) |
| `ENRICH_INTERVAL`        | `24h`                                                            |
| `ENRICH_ALLOW_DIRECT`    | `false`                                                          |
| `ENRICH_FORCE_BOOTSTRAP` | `false`                                                          |

### OpenTelemetry

| Variable                    | Default              |
|------------------------------|----------------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT`| (empty = no-op)       |
| `OTEL_SAMPLE_RATE`          | `1.0`                |

Set `OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4318` (docker) or
`localhost:4318` (local) to enable tracing.
View traces at http://localhost:16686.

### Migrator

| Variable                    | Default                |
|----------------------------|------------------------|
| `MIGRATOR_DSN`            | *(required)*         |
| `MIGRATOR_MIGRATIONS_DIR`  | `/migrations`        |

## Operating the System

### Inspect traces

Open http://localhost:16686 in your browser to view distributed
traces. Each worker span is linked via W3C trace context
propagated through Redis Streams.

### Run a single discovery query

```sh
make ingestion
# Or from the repo root:
docker compose -p dota2-analysis --project-directory . -f deploy/docker-compose.yml \
  run --rm discoverer ./discoverer --file matches
```

The `--file` flag uses the basename of any `.sql` file in
`go-ingestion/assets/queries/<cycle>/` and runs once before exiting.

### Inspect a queue

```sh
make shell-redis
> XLEN dota2:fetch
> XRANGE dota2:parse:dlq - +
```

### Inspect the database

```sh
make shell-db
=> SELECT count(*) FROM matches;
=> SELECT * FROM schema_migrations ORDER BY version;
```

### Add a database migration

1. Create `go-core/schema/migrations/NNN_name.sql` (see the root `Makefile`'s `new-migration` target).
2. `make migrate` (or restart the stack — migrator runs at boot).

### Add a discovery query

1. Drop your SQL into `go-ingestion/assets/queries/<cycle>/<key>.sql`.
2. Restart the discoverer, or run it ad-hoc with `--file <key>`.

### Add an enrichment source

Implement `enrich.RunSource` in
`internal/enrich/sources/<provider>/`, add it to the sources slice in
`cmd/enricher/main.go`, and declare any dependencies through
`DependsOn()`. The runner topologically sorts sources before running.
Reference types come from `refdatastore`; the `enrich` package aliases them
for backward compatibility.

## Development

### Build the binaries locally

```sh
go build ./cmd/...
```

### Run the test suite

```sh
go test ./...
```

Workers are testable without external dependencies via injected interfaces:

- `discovery.HTTPDoer` — for discoverer cycles
- `worker.HTTPDoer` — for fetcher
- `worker.Handler` — generic queue loop (fetcher, parser)
- In-memory adapters (`queue`, `payload`, `dedup`, `metrics`, `proxy`)

### Run the migrator outside Docker

```sh
make migrate-local
```

This expects `POSTGRES_*` env vars to point at a reachable Postgres
instance and `MIGRATOR_MIGRATIONS_DIR` (default `/migrations` inside Docker;
locally use `MIGRATOR_MIGRATIONS_DIR=./go-core/schema/migrations`) to be
readable.