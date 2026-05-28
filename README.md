# Dota 2 Analysis Pipeline

This workspace contains three Go modules that form a Dota 2 data pipeline:

```
dota2-analysis/
├── go-core/          ← Shared module: domain types (Match, HeroID), bootstrap, schema migrations
├── go-analysis/      ← Analytics pipeline (17 features, LambdaMART models, draft recommendations)
├── go-ingestion/     ← Data ingestion pipeline (fetches matches, parses, enriches)
├── deploy/           ← docker-bake.hcl + docker-compose.yml (canonical orchestration)
├── Makefile          ← Unified build/run/test targets
└── go.work           ← Go workspace wiring all three modules
```

## Architecture

```
go-ingestion (writes)          go-analysis (reads)
─────────────────────          ──────────────────
discoverer → fetcher → parser   featurizer → API
  ↓                               ↑       ↓
  Postgres (matches, players,    Postgres (analytics.mv_*)
            picks_bans, …)        ↑
  Redis (queues, proxies)        backtester, recommender
```

Both communicate through Postgres — the schema is managed by `go-core/schema/migrations/`.
`domain.Match` (in `go-core`) serves as the single source of truth for match data across both pipelines.

## Key Technical Highlights

- **Decentralized Config**: Each binary owns its exact configuration boundaries via domain-specific `LoadConfig()` functions. No global god-objects.
- **Pure Queues**: Redis Streams with consumer groups. Payloads are opaque `[]byte` — no JSON introspection or `_retry` hacks. Retry counts are stored as native stream fields.
- **End-to-End Tracing**: OpenTelemetry (OTel) trace context is preserved across the entire pipeline (Fetcher → Redis Queue → Parser → Ingester) via middleware decorators.
- **Env-Driven Log Level**: Set `LOG_LEVEL=debug` across all 11 service binaries via a single env var. `NewLoggerFromEnv()` in `go-core/bootstrap` reads it at startup — no code changes needed to toggle verbosity.
- **Resilient Discoverer**: The matches cycle retries the upstream SQL explorer **indefinitely** until it gets a 200 OK or the service is shut down. Timeouts, 5xx errors, and network blips all trigger retries with exponential backoff (capped at 30s). Each HTTP attempt uses a fresh proxy lease.
- **Decoupled ML Inference**: The Go API server is decoupled from LightGBM via a `ModelReloader` interface. The `ModelWatcher` handles SIGHUP hot-reloads atomically.
- **Per-Candidate Ranking**: The LambdaMART model uses 17 features (8 MV-dependent, 9 per-candidate/draft context) and groups by `(match_id, slot)` to provide true within-decision ranking signal.

## Module Dependencies

```
go-ingestion ──requires──> go-core
go-analysis  ──requires──> go-core
```

**`go-core` must never import downstream.** This is enforced by a contract test (`TestCoreHasNoDownstreamImports`).

## Quick Start

### Prerequisites

- Go 1.26+
- Docker + Docker Compose
- PostgreSQL 16 (for local dev without Docker)

### Local Development

```bash
# Build all modules (workspace-aware)
go build ./go-core/... ./go-analysis/... ./go-ingestion/...

# Test all modules
make test

# Vet all modules
make vet
```

### Full Stack with Docker

```bash
# Build all Docker images
make build

# Start everything (infra → migrate → ingestion → analytics)
make up

# Follow logs
make logs

# Stop everything
make down
```

### Building Specific Groups

```bash
# Build only ingestion images
make build-ingestion

# Build only analysis images
make build-analysis
```

Bake groups correspond to the groups in `deploy/docker-bake.hcl`:

| Group | Targets |
|-------|---------|
| `ingestion` | base, discoverer, fetcher, parser, enricher, proxyloader, dlq, ingestion-migrator |
| `analysis` | base, api, featurizer, backtester |
| `all-images` | ingestion + analysis + migrator (same as default) |

## Module Resolution

The workspace uses:
- **`go.work`** — lists all three modules for local development
- **`replace` directives** in downstream `go.mod` files — point `go-core` to the local directory
- **GOPROXY** — resolves third-party dependencies at build time
- **No `vendor/` directories** — vendoring is not used (see `go-core/ARCHITECTURE.md` for the workspace vendor caveat)

## Schema Contract

- **Owned by ingestion writes**: `public.matches`, `public.players`, `public.player_matches`, `public.picks_bans`, `public.heroes`, etc.
- **Owned by analytics reads/MVs**: `analytics.*` schema
- **Migration policy**: Additive only by default; breaking changes need both-team sign-off
- Migrations are embedded in the migrator binary via `go-core/schema` — no volume mounts needed

### Adding a Migration

```bash
make new-migration NAME=add_foo_column
# Then write the SQL, update contract tests, and PR with both-team approval
```

## Module Boundaries

### go-core

Shared primitives consumed by both projects. See `go-core/README.md` for details.

| Package | Contents |
|---------|----------|
| `domain/` | Typed IDs (HeroID, MatchID, TeamID, etc.) |
| `bootstrap/` | Postgres pool, slog + OTel, telemetry init |
| `config/` | Shared config types (Postgres, Telemetry) |
| `schema/` | Embedded SQL migrations (001_init, 002_analytics, 003_launch_keys) |
| `migrator/` | Embedded-SQL migration runner |
| `contracttest/` | Schema invariant + boundary tests |

### go-ingestion

Seven binaries in `cmd/`: discoverer, fetcher, parser, enricher, proxyloader, migrator, dlq.

### go-analysis

Four binaries in `cmd/`: api, featurizer, backtester, migrator. Plus Python training code in `training/`.

## Typed IDs Policy

Typed IDs (`domain.HeroID`, `domain.MatchID`) should be used at public API boundaries (handler signatures, repository interfaces, exported types). Internal helpers may use raw `int64`/`int` when the type is locally obvious. See `go-core/README.md` for details.

## Makefile Targets

| Target | Description |
|--------|-------------|
| **Build** | |
| `build` | Build all Docker images (root bake file, all groups) |
| `build-ingestion` | Build only ingestion Docker images |
| `build-analysis` | Build only analysis Docker images |
| **Bootstrap** | |
| `bootstrap` | Fetch external data before first run (fetch-constants + fetch-proxies) |
| `fetch-constants` | Download dotaconstants JSON to `go-ingestion/assets/dotaconstants/` |
| `fetch-proxies` | Download proxy list to `go-ingestion/config/proxy.txt` |
| **Lifecycle** | |
| `infra` | Start Postgres + Redis + Jaeger |
| `migrate` | Run database migrations (one-shot) |
| `ingestion` | Start the ingestion pipeline (discoverer, fetcher, parser, enricher, proxyloader) |
| `analytics` | Start the analytics pipeline (api, featurizer) |
| `up` | Full stack: infra → migrate → ingestion → analytics |
| `upd` | Full stack detached |
| `down` | Stop everything |
| `downv` | Stop everything and remove volumes |
| `logs` | Follow all logs |
| **ML** | |
| `backtest` | Run backtester (one-shot) against current model |
| `train` | Train new LightGBM model (one-shot, via go-analysis compose) |
| **Shell** | |
| `shell-db` | Open psql shell |
| `shell-redis` | Open redis-cli |
| **Validate** | |
| `test` | Unit tests for all modules (workspace root) |
| `vet` | go vet for all modules |
| **Schema** | |
| `new-migration` | Scaffold a new migration file (NAME=snake_case) |
| **Publish** | |
| `publish-core` | Tag and push go-core module |
| **Other** | |
| `vendor` | No-op — explains module resolution |
| `armageddon` | ⚠️ Nuke all Docker resources for this project |
| `help` | Show all targets |

## Docker Images

All images are built via `deploy/docker-bake.hcl` with the `go-dota2-*` naming convention:

| Image | Source |
|-------|--------|
| `go-dota2-ingestion-base` | Builder with all ingestion binaries |
| `go-dota2-discoverer` | Match discovery service |
| `go-dota2-fetcher` | Match data fetcher |
| `go-dota2-parser` | Match parser |
| `go-dota2-enricher` | Match enricher |
| `go-dota2-proxyloader` | Proxy rotation service |
| `go-dota2-dlq` | Dead letter queue handler |
| `go-dota2-ingestion-migrator` | Ingestion-specific migrator |
| `go-dota2-migrator` | Canonical migrator (used by `docker-compose.yml`) |
| `go-dota2-analysis-base` | Builder with all analysis binaries |
| `go-dota2-analysis-api` | Draft recommendation API |
| `go-dota2-analysis-featurizer` | Feature computation service |
| `go-dota2-analysis-backtester` | Backtesting (one-shot) |

### Build Cache

The bake file supports GitHub Actions cache:
```bash
docker buildx bake -f deploy/docker-bake.hcl --load \
  --set *.cache-from=type=gha \
  --set *.cache-to=type=gha,mode=max
```

## CI/CD

The monorepo uses a unified GitHub Actions workflow (`.github/workflows/ci.yml`) that:

- Builds and tests all three modules in a matrix
- Runs contract tests against real Postgres (service container)
- Lints all modules with golangci-lint
- Enforces module boundaries (`TestCoreHasNoDownstreamImports`)
- Builds Docker images on main branch after tests pass
- Runs Renovate bot weekly for dependency updates
