# go-dota2-analysis

Draft analysis engine for Dota 2 — part of the `dota2-analysis` monorepo.

## Overview

`go-analysis` is read-side analytics: it reads from Postgres (`public.*` ingestion tables + `analytics.*` materialized views), computes feature vectors, scores heroes, and serves draft recommendations via HTTP.

## Binaries

| Binary | Path | Purpose |
|--------|------|---------|
| `api` | `cmd/api/` | HTTP API for draft recommendations, team/player/hero profiles |
| `featurizer` | `cmd/featurizer/` | Periodic refresh of materialized views + snapshots |
| `backtester` | `cmd/backtester/` | One-shot historical draft evaluation |
| `migrator` | `cmd/migrator/` | Runs embedded schema migrations from `go-core` |

## Internal Packages

| Package | Purpose |
|---------|---------|
| `api/` | HTTP server, handlers, DTOs, middleware (auth, logging, request-id) |
| `bootstrap/` | Postgres pool, OTel telemetry, logger (thin wrapper over `go-core`) |
| `config/` | Env-driven typed config |
| `domain/` | Draft state machine, feature vectors, scores, phase table |
| `eval/` | Backtesting framework and baselines |
| `features/` | Feature builder with pluggable sources + registry |
| `featurize/` | Periodic MV refresher |
| `profiles/` | Repository interface for all data access |
| `recommend/` | Recommendation service + ensemble scoring |
| `scoring/` | Scorer interface: linear and LGBM implementations |
| `storage/postgres/` | Full repository implementation (Postgres MVs) |

## Quick Start

```bash
# Build binaries
go build ./cmd/...

# Run tests
go test ./...

# Run all tests (workspace-aware, from repo root)
make test
```

## Module Dependencies

```
go-analysis  ──requires──>  go-core (shared domain types, bootstrap, migrator)
```

## Data Flow

1. **Sibling ingestion** writes raw match data to `public.*`
2. **Featurizer** refreshes `analytics.*` materialized views (every 24h)
3. **API** reads from MVs, scores candidates, serves recommendations
4. **Trainer** (Python) trains LightGBM models offline
5. **Backtester** evaluates models against historical drafts

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/health` | Service health + featurizer staleness |
| `POST` | `/v1/recommend` | Draft hero recommendations |
| `POST` | `/v1/draft/simulate` | Full draft simulation |
| `GET` | `/v1/teams/{id}/profile` | Team hero history |
| `GET` | `/v1/h2h` | Head-to-head team comparison |
| `GET` | `/v1/heroes/{id}/synergy` | Hero synergy partners |
| `GET` | `/v1/heroes/{id}/counter` | Hero counters |
| `GET` | `/v1/players/{id}/profile` | Player hero + team history |

## Scoring

Two scorer backends:

- **Linear**: Hand-tuned weighted sum of 8 feature sources. Default, no model files needed.
- **LGBM**: LightGBM model loaded from `assets/models/imitation/current/`. SIGHUP hot-reload.

Switch via `ANALYTICS_SCORER_KIND=linear|lgbm`.

## ML Training

Python package in `training/`:

```bash
# Extract data → train imitation model → evaluate → publish
trainer all
# Or step by step:
trainer extract
trainer train-imitation
trainer evaluate
trainer publish
```

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) — detailed deployment runbook and patch transition guide
- `go-core/` — shared domain types, bootstrap, and schema migrations
- `go-ingestion/` — data ingestion pipeline (sibling project)
