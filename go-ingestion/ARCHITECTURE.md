# Architecture

`go-ingestion` is a distributed match-data ingestion pipeline composed of small,
single-purpose binaries that communicate through Redis. This document describes
the system's structure, data flow, and the rationale behind key design choices.

## Table of Contents

- [System Overview](#system-overview)
- [Pipeline Stages](#pipeline-stages)
- [Package Layout](#package-layout)
- [Data Flow](#data-flow)
- [Storage Model](#storage-model)
- [Cross-Cutting Concerns](#cross-cutting-concerns)
- [Design Principles](#design-principles)
- [Failure Handling](#failure-handling)
- [Configuration](#configuration)
- [Extending the System](#extending-the-system)

## System Overview

The system is a **staged pipeline**. Each stage is an independent process that
consumes from one Redis Stream, performs a focused unit of work, and (optionally)
produces work for the next stage.

```
┌──────────────┐    ┌─────────────┐
│ proxyloader  │───▶│ proxy pool  │ (Redis ZSET, ranked)
└──────────────┘    └──────┬──────┘
                           │ leased by all HTTP workers
                           ▼
┌──────────────┐    ┌──────────────┐    ┌─────────┐    ┌──────────────┐    ┌─────────┐    ┌────────────┐
│  discoverer  │───▶│ fetch queue  │───▶│ fetcher │───▶│ parse queue  │───▶│ parser  │───▶│  Postgres  │
└──────────────┘    └──────────────┘    └────┬────┘    └──────────────┘    └────┬────┘    └────────────┘
                                             │                  ▲
                                             ▼                  │
                                    ┌──────────────┐           │
                                    │ payload blob │───────────┘
                                    │ store (Redis)│
                                    └──────────────┘

┌──────────┐   ┌────────────┐
│ enricher │──▶│  Postgres  │  (heroes, items, patches, …)
└──────────┘   └────────────┘

┌──────────┐   ┌────────────┐
│ migrator │──▶│  Postgres  │  (one-shot, applies SQL migrations)
└──────────┘   └────────────┘

┌────────┐   ┌────────────┐
│ Jaeger │◀──│ OTLP/4318  │  (traces + metrics)
└────────┘   └────────────┘
```

## Pipeline Stages

### `proxyloader`

Loads proxies from a seed file (`proxy.txt`) and an optional remote source,
validates each one against a canary URL (e.g. `https://api.ipify.org`), and
publishes healthy proxies to a Redis ZSET. Validation runs in chunks with
bounded parallelism so the pool can start serving requests as soon as the
first chunk is verified.

After the initial load, it runs two independent refresh cycles:

- **Top-up** (`PROXY_REFRESH_INTERVAL`) — conditional reload only when the
  pool drops below `PROXY_MIN_POOL_SIZE`. Handles sudden eviction bursts.
- **Force-refresh** (`PROXY_FORCE_REFRESH_INTERVAL`) — unconditional
  re-validation of the full proxy list. Evicts degraded proxies before
  they hit the `PROXY_MAX_FAILURES` threshold. Defaults to `1h`; set to
  `0` to disable.

### `discoverer`

Reads `.sql` files from a configured directory, sends each query to an
explorer-style endpoint (e.g. OpenDota's `/api/explorer`), and pushes the
returned match IDs onto the **fetch queue**. Supports a one-shot mode
(`--file <key>`) for ad-hoc backfills and a scheduled mode driven by
`DISCOVERY_INTERVAL`.

Each discovery cycle uses the
`discovery.HTTPDoer` interface, allowing injected HTTP clients for
testing without a proxy pool. Currently only the **matches** cycle has
an implementation (`internal/worker/discovery/matches/`); leagues, teams,
and proplayers cycles are planned but not yet coded.

#### PostgreSQL Pre-Filtering (Redis-Flush Resilience)

The matches cycle consults PostgreSQL **before** the Redis dedup layer.
After fetching candidate match IDs from the explorer API, the cycle calls
`matchstore.MatchReader.UnknownIDs()` to filter out matches that either
do not exist yet in the database or have `is_parsed = FALSE`. This means
that even if Redis is flushed (wiping the `dedup` seen-set), the
discoverer will not re-enqueue matches that are already fully parsed in
PostgreSQL — the true source of truth.

```
Explorer API → fetchMatchIDs() → DB UnknownIDs filter → Redis dedup → Queue
                                      ↑                       ↑
                               PostgreSQL source      ephemeral cache
                               of truth                (can be flushed)
```

The DB connection is established whenever `POSTGRES_DSN` is set, regardless
of whether other DB-backed cycles (leagues, teams, pro-players) are
enabled. If the connection fails, the discoverer logs a warning and
operates without DB filtering, relying solely on the Redis dedup set.

Creates a root `cycle.run` span in OpenTelemetry to enable end-to-end
trace visualization.

### `fetcher`

Pops match-ID tasks from the fetch queue, fetches the raw match JSON from the
upstream API through the injected `worker.HTTPDoer`, stores the blob in the
**payload store** (Redis with TTL), and pushes a "ready to parse" task onto
the **parse queue**.

The caller (typically `cmd/fetcher/main.go`) composes the HTTPDoer with proxy
configuration — the fetcher itself is testable without a real proxy pool.
It uses the generic `worker.Run()` loop with `worker.Handler`.

Permanent HTTP errors (404, 401, 403, 410, 418) are dropped immediately;
transient errors trigger a different proxy on retry. Rate-limit errors
(`429`) are requeued without penalising the proxy.

Creates a `worker.process` span inheriting the trace context from the Redis Stream.

### `parser`

Pops from the parse queue, retrieves the blob from the payload store, decodes
and validates the match JSON, and hands the validated `Match` to the **ingester**.
On success, the blob is deleted from the payload store and the task is acked.

The parser uses the generic `worker.Run()` loop with `worker.Handler`,
the same pattern as the fetcher.

Creates a `worker.process` span inheriting the trace context from the Redis Stream.

### `ingester`

In-process collaborator of the parser. Persists the match via the
`matchstore.MatchWriter` and marks the match ID as seen in the dedup set.
Currently a thin wrapper but kept separate so persistence policy can evolve
independently of decoding.

### `enricher`

Periodically refreshes static reference data (heroes, abilities, items,
patches, game modes, lobby types, regions) from the
[`odota/dotaconstants`](https://github.com/odota/dotaconstants) repository
and upserts it into Postgres. Sources are topologically sorted so that
dependencies (e.g. `hero_abilities` after `heroes`) run in the correct
order. A `RunGate` prevents redundant runs within the configured interval.

The gate can be bypassed by setting `ENRICH_FORCE_BOOTSTRAP=true`, which
replaces the interval-based gate with an `Always{}` gate. This is useful
for recovery scenarios — e.g. when the enricher was restarted mid-interval
and the Redis gate keys are still fresh, blocking the at-start run. Set it
for one restart, then revert to `false` once the data is populated.

Sources implement the `enrich.RunSource` interface in `enrich/sources/` —
adding a new source requires implementing the interface and registering it
in the sources slice in `cmd/enricher/main.go`. The `refdatastore` package provides the canonical
types (`MatchRef`, `HeroRef`, etc.); `enrich` aliases these for
backward compatibility. The `initmarker` package provides
`BootstrapMarker` for source gating.

### `dlq`

Consumes messages from dead-letter streams (`dota2:fetch:dlq`, `dota2:parse:dlq`)
after they exceed `QUEUE_MAX_RETRIES`. Supports three commands via `--cmd`:

- **`list`** — print all DLQ messages as formatted JSON
- **`requeue`** — return messages to their origin stream for reprocessing
- **`purge`** — delete all messages from the DLQ

This binary is a standalone debugging and recovery tool — it is not part of the
main pipeline and does not run by default.

### `migrator`

Runs once at deploy time. Discovers numbered SQL files in
`go-core/schema/migrations/`, compares them against the `schema_migrations` table,
and applies any pending migrations inside a transaction.

## Package Layout

```
cmd/                    Process entry points (one main per binary)
  discoverer/           Match-ID discovery with retry logic
  fetcher/              HTTP ingestion via proxies
  parser/               Decoding + validation + ingest
  enricher/             Static reference data
  proxyloader/          Proxy pool maintainer
  migrator/             SQL migrations
  dlq/                  Dead-letter queue inspector & recovery

internal/
  bootstrap/            Wires Redis/Postgres/metrics/proxy from Config
  config/               Env-driven Config struct with all settings

  dedup/                Seen-set abstraction (inmem + redis)
  dlq/                  Dead-letter queue commands (list, requeue, purge)
  enrich/               Reference-data domain
    httpclient/         HTTPClient interface + implementations
    initmarker/         BootstrapMarker interface
    gate/               RunGate (always / once / interval)
    sources/            One file per dotaconstants resource

  metrics/              Sink (inmem + otelmetrics + noop)
  payload/              Blob store (inmem + redisstore)
  proxy/                Lease-based pool
    httpdo/             OTel-wrapped HTTPDoer for trace injection

  queue/                Push/Pop/Ack/Retry abstraction
    redisstreams/       Queue with W3C trace propagation

  storage/              Ports-and-adapters
    pgclient/           Pool opener + Stores bundle + otelpgx tracer
    matchstore/         MatchWriter/MatchReader interfaces + matchpg adapter
      match.go          Core Match type
      draft.go          Pick/ban draft types
      environment.go    Game environment (game mode, lobby, region)
      events.go         Timeline event types
      identity.go       Player identity (slot, team, account)
      interfaces.go     MatchWriter / MatchReader interfaces
      players.go        Per-player statistics
      teams.go          Team-level aggregates
    lookupstore/        LookupReader interface + lookuppg adapter
    partitionstore/     PartitionAdmin interface + partitionpg adapter
    refdatastore/       RefDataWriter interface + refdatapg adapter
    redis/              Connection wrapper

  worker/               Pipeline implementations
    discoverer/
    fetcher/
    parser/
      decode.go         Core Match type and top-level decode
      decode_match.go   Match-level JSON fields
      decode_players.go Player array and per-player fields
      decode_draft.go   Pick/ban phase reconstruction
      decode_events.go  Timeline events (runes, kills, objectives)
    runner.go           Generic queue runner + Handler/HTTPDoer interfaces + OTel spans
```

The repository follows a **ports-and-adapters** layout: each domain capability
lives behind a small interface (a "port") in its own package, with concrete
implementations ("adapters") in subpackages (e.g., `matchpg`, `lookuppg`,
`refdatapg`). Workers depend only on the interfaces, not on the adapters.

## Data Flow

A single match traverses the system as follows:

1. `discoverer` runs an SQL query, gets `match_id = 12345`. Before pushing
   to the queue, it checks PostgreSQL via `UnknownIDs()` to confirm the
   match either doesn't exist or has `is_parsed = FALSE`. If already
   parsed, the match is skipped. Otherwise `{"match_id": 12345}` is
   pushed to `dota2:fetch`. The W3C `traceparent` header is injected
   into the Redis Stream message.
2. `fetcher` pops the task, extracts the trace context, and creates a
   child span `worker.process`. It leases a proxy from `dota2:proxy:set`,
   and `GET`s `<UPSTREAM>/12345` via `otelhttp` (creating another child span).
3. The raw JSON body is stored at `dota2:payload:12345` with a TTL, and a
   new task is pushed to `dota2:parse` with the trace context preserved.
4. `parser` pops the task, extracts the trace context, creates a child
   span, fetches the blob, decodes and validates it, and calls the ingester.
5. The ingester writes a row into `matches` (Postgres) via `otelpgx` (another
   child span) and marks `12345` in the dedup set.
6. The blob is deleted, both queue messages are acked.

Failures at any step go through a **retry policy** with exponential
backoff and jitter; after `QUEUE_MAX_RETRIES` attempts the task is moved
to the corresponding DLQ stream.

## Storage Model

- **Postgres** — durable store for matches and reference data. Schemas
  live in `go-core/schema/migrations/`. Storage uses a **ports-and-adapters** layout:

  | Port | Adapter | Purpose |
  |------|---------|---------|
  | `matchstore.MatchWriter/MatchReader` | `matchpg.Store` | Match ingest & queries |
  | `lookupstore.LookupReader` | `lookuppg.Store` | Hero/Item IDs, patch lookup |
  | `partitionstore.PartitionAdmin` | `partitionpg.Admin` | Time partitioning |
  | `refdatastore.RefDataWriter` | `refdatapg.Store` | Heroes, items, patches, etc. |

  All adapters are constructed via `pgclient.Stores`, which bundles them
  for convenient wiring. Workers depend only on the interfaces they need.

  Uses **otelpgx** for automatic tracing of all database operations.

- **Redis** — operational state:
  - Streams for queues (`dota2:fetch`, `dota2:parse` and their DLQs)
  - ZSET for the proxy pool with score = ranking weight
  - HASH per proxy for stats (`success`, `fail`, `consecutive_fail`, …)
  - Strings for payload blobs (TTL-bounded)
  - Strings/SET for the dedup seen-set
  - Strings for enrich gating (last-run timestamps)

## Cross-Cutting Concerns

### OpenTelemetry (Tracing + Metrics)

All services initialize a `TracerProvider` via `bootstrap.InitTelemetry()` which:

- Creates an OTLP HTTP trace exporter (pointing to `OTEL_EXPORTER_OTLP_ENDPOINT`)
- Uses a `ParentBased(TraceIDRatioBased(sampleRate))` sampler
- Sets the W3C `TraceContext` and `Baggage` propagators

All HTTP clients use `otelhttp.NewTransport()` for automatic span creation
and trace context propagation.

All database operations use `otelpgx` for automatic span creation.

Redis Streams propagate W3C trace context via `_otel_*` prefixed fields:
- `Push()` injects the context into the message
- `decodeMessage()` extracts the context and attaches it to `queue.Task.Ctx`

The discoverer creates a root `cycle.run` span; workers create `worker.process`
spans that automatically nest under the trace context from the queue.

View traces at **http://localhost:16686** (Jaeger UI).

### Metrics

`metrics.Sink` is implemented by:
- `otelmetrics.Sink` — pushes counters to OTel (production)
- `inmem.Sink` — in-memory counters (testing)
- `noop.Sink` — no-op (testing)

Counters for ingest/parse/fetch success/failure are incremented in-line;
`failures_by_kind` is an enum (`decode`, `validate`, `db`, `http`, `timeout`,
`rate_limit`, `not_found`, `proxy`, `payload`, `unknown`).

### Proxy Pool

Atomic operations are implemented in **embedded Lua scripts** (`internal/proxy/redisproxy/lua/*.lua`):

- `acquire.lua` — picks the highest-ranked free proxy, marks it leased
  with TTL.
- `release.lua` — releases by token.
- `rate_limit.lua` — global token-bucket-style limiter.
- `record_success.lua` / `record_failure.lua` — adjust rank, evict
  after N consecutive failures.

Each acquisition returns a `*proxy.Lease` carrying callbacks for
`Release`, `MarkSuccess`, and `MarkFailure`. Double-release is guarded
by `atomic.Bool`.

The `httpdo.Doer` acquires a **fresh proxy lease per retry attempt**
inside its retry loop. This means a transport-level failure (TLS error,
connection refused, EOF) on one proxy does not cause all remaining
retries to hit the same broken proxy. TLS/x509 errors are classified
as proxy faults and trigger an immediate proxy switch without backoff,
since the connection timeout was already paid.

Transports are produced by `internal/proxy/transport/transport.go`,
which supports HTTP, HTTPS, SOCKS5, and SOCKS5h via
`golang.org/x/net/proxy`.

### Queue (Redis Streams)

`redisstreams.Queue` wraps `XADD` / `XREADGROUP` / `XACK` /
`XAUTOCLAIM`. A consumer group decouples concurrent readers, and
`MaxLen` keeps the stream bounded.

`Retry()` increments the retry count, applies a quadratic backoff with
jitter, and either re-adds the message or routes to the DLQ once
`MaxRetries` is exceeded. `RecoverStale()` reclaims pending messages
from crashed consumers.

W3C trace context is automatically propagated through queue messages
via `_otel_traceparent` and `_otel_tracestate` fields.

> **Note:** during a retry's backoff the original message is still
> pending in the consumer group. A worker crash mid-backoff results in
> redelivery via `XAUTOCLAIM`, which can produce a duplicate when
> combined with the requeue. The pipeline tolerates this through
> `ON CONFLICT DO NOTHING` and the dedup set.

### Dedup

`dedup.Seen` is a small contract:

```go
MarkSeen(ctx, key) (alreadySeen bool, err error)
IsSeen(ctx, key)   (bool, error)
```

The Redis implementation uses either a `SET` member (no TTL) or per-key
`SETNX` (TTL).

#### Two-Layer Dedup in the Discoverer

The matches cycle uses a two-layer dedup strategy. The **primary** check
is PostgreSQL via `matchstore.MatchReader.UnknownIDs()`: match IDs
already in the database with `is_parsed = TRUE` are filtered out before
any Redis operation. The **secondary** check is the Redis `dedup.Seen`
set, which catches matches already enqueued in the current cycle's
lifetime. This layered approach tolerates a Redis flush — the only
consequence is a single extra `SELECT` query to PostgreSQL for each
discovery cycle, rather than mass re-enqueuing of already-processed
matches.

### Bootstrap & Wait

`internal/bootstrap` centralises wiring. Each `cmd/<binary>` calls a
small set of helpers (`Core`, `ProxyPool`, `FetchQueue`, …) so all
binaries share identical configuration parsing and connection setup.
`WaitForProxies` and `WaitForPostgres` block startup until external
dependencies are reachable.

## Design Principles

**Ports and adapters.** Every cross-process collaborator is reached
through an interface in `internal/<domain>` with adapters in
subpackages. Storage adapters (`matchpg`, `lookuppg`, `partitionpg`,
`refdatapg`) implement the port interfaces; workers depend on the
interfaces, not on the adapters.
**Small, restartable processes.** Each binary is stateless and idempotent.
Restarting any worker is safe — in-flight messages are reclaimed via
`XAUTOCLAIM` and duplicates are absorbed by the dedup set plus
database uniqueness constraints.
**Backpressure through bounded queues.** `MaxLen` on Redis Streams
prevents unbounded growth when downstream stages slow down.
**Fail loud, fail typed.** Errors are classified into a closed set of
`metrics.FailureKind`s so dashboards and alerts can distinguish
"upstream is rate-limiting us" from "decoding broke after a schema
change".
**Configuration via environment.** All settings are loaded into typed config
structs in `internal/config/config.go`. Workers receive configured instances
— no direct `os.Getenv` calls in worker packages. This improves testability
and ensures all settings are in one place.
**Observability via OpenTelemetry.** All services push traces and metrics via
OTLP to Jaeger. W3C trace context propagates through Redis Streams
for end-to-end visualization.

## Primitives

| Primitive      | Type                | Flows Through                              |
|----------------|---------------------|-------------------------------------------|
| MatchID        | int64               | discoverer → fetch queue → fetcher → parser → matchstore |
| RawPayload     | []byte (JSON)       | fetcher → payload store → parser          |
| Match          | matchstore.Match    | parser → matchstore.MatchWriter           |
| MatchRef       | refdatastore.MatchRef | parser → refdatastore                   |
| Proxy          | proxy.Proxy         | proxyloader → proxy pool → fetcher       |
| HTTPDoer       | worker.HTTPDoer     | fetcher (injected)                     |
| BootstrapMarker| initmarker.BootstrapMarker | enricher → source gating              |
| PatchInfo      | lookupstore.PatchInfo | fetcher/parser → lookupstore.LookupReader |
| QueueTask      | queue.Task          | redisstreams queue + trace context        |

**Type origins:**

- `MatchID` — from upstream API (OpenDota), canonical identifier
- `RawPayload` — JSON blob from `<UPSTREAM>/match/<id>`
- `Match` — parsed and validated domain object with Players, Details, Draft, etc.
- `MatchRef` — reference from refdatastore for hero/item name resolution
- `Proxy` — leased from Redis ZSET, carries endpoint + credentials
- `HTTPDoer` — discovered or injected, used by cycles and fetcher
- `PatchInfo` — resolved from timestamp via `PatchByTimestamp()`

## Failure Handling

| Stage      | Failure                          | Action                                 |
|------------|----------------------------------|----------------------------------------|
| fetcher    | 4xx permanent (404/401/403/410)  | Drop + ack                              |
| fetcher    | 429 / 5xx                         | Mark proxy failure, retry on new proxy  |
| fetcher    | Network / timeout                 | Mark proxy failure, retry on new proxy  |
| fetcher    | TLS/x509 (proxy clock-skew)      | Mark proxy failure, switch proxy immediately (no backoff) |
| fetcher    | Payload store error               | Retry the queue task                    |
| parser     | Payload missing (TTL expired)     | Drop + ack                              |
| parser     | Decode/validate error             | Retry; eventually DLQ                   |
| parser     | DB error in ingester              | Retry; eventually DLQ                   |
| enricher   | Source non-critical               | Log warn, continue                      |
| enricher   | Source critical                   | Abort cycle                             |
| any        | Redis/Postgres outage             | Workers loop with backoff               |

After `QUEUE_MAX_RETRIES` failed attempts, messages are moved to a DLQ
stream (`dota2:fetch:dlq`, `dota2:parse:dlq`) for manual inspection.

## Configuration

All configuration is environment-driven and lives in
`internal/config/config.go`. Highlights:

- `REDIS_ADDRS`, `REDIS_PASSWORD`, `REDIS_DB`, pool sizing
- `POSTGRES_DSN` and connection limits
- `PROXY_*` — pool size, hold time, ranking weights, validation parallelism,
  force-refresh interval
- `QUEUE_*` — group name, `MAX_LEN`, retry policy
- `DISCOVERY_QUERIES_DIR`, `DISCOVERY_DEFAULT_KEY`, `DISCOVERY_RUN_AT_START`
- `FETCHER_*` — upstream URL, batch size, timeouts, payload TTL
- `ENRICH_DOTACONSTANTS_BASE_URL`, `ENRICH_FORCE_BOOTSTRAP`
- `MIGRATOR_DSN`, `MIGRATOR_MIGRATIONS_DIR`
- `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SAMPLE_RATE`

Defaults are sensible for a single-host development setup; production
deployments override via `.env` or the orchestration layer.

## Extending the System

**Adding a new enrich source.** Implement `enrich.RunSource` in
`internal/enrich/sources/<provider>/`, add it to the sources slice in
`cmd/enricher/main.go`, and declare any dependencies through
`DependsOn()`. The runner's topological sort will schedule it correctly.
Types come from `refdatastore` — the canonical package for reference data.

**Adding a new pipeline stage.** Define a queue spec in
`internal/bootstrap/bootstrap.go`, write a worker under
`internal/worker/<name>/` that implements `worker.Handler` (see
`fetcher` and `parser` for examples), and wire it in
`cmd/<name>/main.go` via `worker.Run()`. Failures should classify into
existing `metrics.FailureKind`s where possible.

**Swapping a backend.** Each port has at least an in-memory adapter
useful for tests. To replace Redis Streams with, say, NATS JetStream,
implement `queue.Queue` in a new package and update the bootstrap
helpers — no worker code changes.

**Adding a database column.** Create a new numbered file in
`go-core/schema/migrations/` (`make new-migration NAME=add_column`
from the repo root). The next `migrator` run picks it up automatically.