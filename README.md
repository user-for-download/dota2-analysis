# Dota 2 Analysis Pipeline

This workspace contains three Go modules that form a Dota 2 data pipeline:

```
dota2-analysis/
├── go-core/          ← Shared module: domain types, bootstrap, schema migrations, migrator
├── go-ingestion/     ← Data ingestion pipeline (fetches matches, parses, enriches)
└── go-analysis/      ← Analytics pipeline (features, models, draft recommendations)
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

## Module: go-core

Shared primitives consumed by both projects:

| Package | Contents |
|---------|----------|
| `domain/` | Typed IDs (HeroID, MatchID, TeamID, etc.) |
| `bootstrap/` | Postgres pool, slog + OTel, telemetry init |
| `config/` | Shared config types (Postgres, Telemetry) |
| `schema/` | Embedded SQL migrations (001_init, 002_analytics) |
| `migrator/` | Embedded-SQL migration runner |
| `contracttest/` | Schema invariant tests |

## Module: go-ingestion

Seven binaries in `cmd/`: discoverer, fetcher, parser, enricher, proxyloader, migrator, dlq.

## Module: go-analysis

Four binaries in `cmd/`: api, featurizer, backtester, migrator. Plus Python training code in `training/`.

## Development

### Prerequisites

- Go 1.26+
- Docker + Docker Compose
- PostgreSQL 16 (for local dev without Docker)

### Quick Start

```bash
# Vendor dependencies (required before Docker builds)
cd go-ingestion && make vendor
cd go-analysis && make vendor

# Start databases
cd go-ingestion && make up-db

# Run migrations
cd go-ingestion && make migrate

# Start ingestion pipeline
cd go-ingestion && make up

# In another terminal, start analytics
cd go-analysis && make up
```

### Production Builds

```bash
# Build all Docker images
cd go-ingestion && make build
cd go-analysis && make build
```

Images use `-mod=vendor` for reproducible builds without external module proxy access.

## Module Dependencies

```
go-ingestion ──requires──> go-core
go-analysis  ──requires──> go-core
```

Both projects vendor `go-core` via `go mod vendor`. The `replace` directive in `go.mod` is for local development only — Docker builds use vendor exclusively.

## Schema Contract

- **Owned by ingestion writes**: `public.matches`, `public.players`, `public.player_matches`, `public.picks_bans`, `public.heroes`, etc.
- **Owned by analytics reads/MVs**: `analytics.*` schema
- **Migration policy**: Additive changes only without RFC; breaking changes need both-team sign-off

Migrations are embedded in the migrator binary via `go-core/schema`. No volume mounts needed.
