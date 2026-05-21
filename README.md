# Dota 2 Analysis Pipeline

This workspace contains three Go modules that form a Dota 2 data pipeline:

```
dota2-analysis/
├── go-core/          ← Shared module: domain types, bootstrap, schema migrations, migrator
├── go-analysis/      ← Analytics pipeline (features, models, draft recommendations)
├── go-ingestion/     ← Data ingestion pipeline (fetches matches, parses, enriches)
├── deploy/           ← Shared deployment artifacts (dotaconstants, queries, bake config)
├── docker-compose.yml ← Canonical orchestration for the full stack
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
go test ./go-core/... -short
go test ./go-analysis/... -short
go test ./go-ingestion/... -short

# Start full stack via Docker
docker compose up -d
```

### Production Builds

```bash
# Build all Docker images
docker buildx bake -f deploy/docker-bake.hcl --load

# Or per-project:
cd go-ingestion && docker buildx bake --file deploy/docker-bake.hcl --load
cd go-analysis  && docker buildx bake --file deploy/docker-bake.hcl --load
```

## Module Resolution

The workspace uses:
- **`go.work`** — lists all three modules for local development
- **`replace` directives** in downstream `go.mod` files — point `go-core` to the local directory
- **GOPROXY** — resolves third-party dependencies at build time
- **No `vendor/` directories** — vendoring is not used

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
| `schema/` | Embedded SQL migrations (001_init, 002_analytics) |
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
| `build` | Build all Docker images |
| `test` | Unit tests for all modules |
| `vet` | go vet for all modules |
| `migrate` | Run schema migrations |
| `new-migration` | Scaffold a new migration file |
| `up` | Start full stack |
| `down` | Stop full stack |
| `logs` | Follow all logs |
| `publish-core` | Tag and publish go-core module |

## CI/CD

The monorepo uses a unified GitHub Actions workflow (`.github/workflows/ci.yml`) that:

- Builds and tests all three modules in a matrix
- Runs contract tests against real Postgres
- Lints all modules with golangci-lint
- Builds Docker images on main branch after tests pass
