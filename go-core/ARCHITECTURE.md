# go-core Architecture

## Rationale

`go-core` exists to eliminate drift between the ingestion and analytics pipelines. Before the monorepo, each project maintained its own copy of:

- Database connection setup and pooling
- Logger initialization with OTel trace injection
- Telemetry (OpenTelemetry) initialization
- Schema migrations (which inevitably diverged — `002_analytics.sql` had different GRANTs)

`go-core` is the single source of truth for these cross-cutting concerns.

## What Belongs in go-core

- **Domain types** shared between ingestion and analytics (hero IDs, match IDs, etc.)
- **Bootstrap infrastructure** — Postgres, logger, telemetry
- **Schema migrations** — all SQL migrations
- **Migration runner** — the `migrator.Run` function
- **Contract tests** — schema invariant tests that enforce correctness

## What Does NOT Belong in go-core

- **Redis clients or queue infrastructure** — ingestion-specific
- **HTTP clients or upstream API interaction** — ingestion-specific
- **ML models or scoring** — analytics-specific
- **Business logic** — match parsing, enrichment, drafting, recommending
- **Any import of `go-analysis` or `go-ingestion`** — enforced by boundary test

## Decision Log

### Why DISCARD TEMP, not DISCARD ALL?

The `pgclient` package (inherited from ingestion) uses `DISCARD TEMP` instead of `DISCARD ALL` to reset connection state. `DISCARD ALL` would also discard prepared statements, which degrades performance. `DISCARD TEMP` is sufficient to clean temporary tables between connections.

### Why is migrator a library, not a binary?

Each downstream project has its own entrypoint (`cmd/migrator/main.go`) that imports the shared `migrator.Run` function. This allows per-project concerns (config loading, signal handling, telemetry setup) to live in the project while the migration logic is shared. The wrappers are intentionally thin (~40-70 lines).

### Why advisory lock 13625?

The original ingestion used 13624 and analysis used 13625. The canonical version is 13625 (tracked as a constant in `migrator/runner.go`). If you need to run two migrators concurrently, they must use distinct lock IDs.

### Why type aliases (`=`), not type definitions?

```go
type HeroID = core.HeroID  // alias — transparent to type system
// vs
type HeroID core.HeroID    // new type — requires explicit conversion
```

Aliases allow zero-cascade adoption: existing code using `int64` continues to compile since aliases don't introduce new types. This was critical for the opportunistic adoption strategy.
