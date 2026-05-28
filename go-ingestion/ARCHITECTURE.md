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

Each discovery cycle uses the `discovery.HTTPDoer` interface, allowing
injected HTTP clients for testing without a proxy pool. Currently only
the **matches** cycle has an implementation
(`internal/worker/discovery/matches/`); leagues, teams, and proplayers
cycles are planned but not yet coded.

#### Retry Logic: "Until 200"

The matches cycle's inner `fetchMatchIDs()` call uses `httpdo.Doer` for
proxy-aware HTTP execution. The doer makes up to `DISCOVERY_MAX_RETRIES`
attempts (one fresh proxy lease per attempt), rotating proxies on timeouts
and 5xx errors. Non-200 status codes are treated as permanent failures
(4xx) or retried (5xx) at the doer level.

If the doer exhausts all proxy retries or the API returns a non-200
response, the **outer retry loop** in `Cycle.RunOnce()` kicks in. Unlike
a fixed-retry-count approach, this loop retries indefinitely — it only
exits when:
- `fetchMatchIDs()` returns a **200 OK** with valid JSON → **success**
- The **context is cancelled** (SIGTERM/SIGINT → service shutdown)

Backoff is exponential (`RetryBackoff × attempt`), **capped at 30s**, so
the discoverer does not spam a downed upstream but still recovers quickly
when the API comes back online.

```
Outer loop (retry until ctx cancelled or 200 OK):
  ├── doer loop (up to MaxRetries × proxy)
  │     ├── proxy A → 30s timeout → switch proxy
  │     ├── proxy B → 5xx → switch proxy
  │     └── proxy C → 200 OK → return response
  ├── fetchMatchIDs checks: resp.StatusCode == 200?
  │     ├── YES → parse body, return IDs
  │     └── NO  → wrap as retryable error
  └── backoff: 500ms → 1s → 1.5s → … → capped at 30s
```

#### HTTP Timeout

Each individual HTTP call is capped by `DISCOVERY_HTTP_TIMEOUT` (default
**180s**, configurable). The OpenDota SQL explorer endpoint is
notoriously slow — the generous timeout gives it room to complete while
still failing fast on a truly dead upstream.

#### Status Code Validation

`fetchMatchIDs()` explicitly checks `resp.StatusCode == 200`. A non-200
response (e.g. a 503 with an error body) is wrapped as a retryable error
rather than silently dropped or parsed as valid JSON.

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

#### LocalURL Fast-Path

When `FETCHER_UPSTREAM_LOCAL_URL` is set (e.g.
`http://ocode-app:8080/api/v1`), the fetcher creates a **local HTTP client**
that bypasses the proxy pool entirely. The `LocalURL` is preferred over the
proxy-backed `UpstreamURL` when non-empty. This is used in local development
where the upstream API mirror runs on the same Docker network, making proxy
rotation unnecessary. The standard `UpstreamURL` is still used when
`LocalURL` is empty (production).

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

When `ENRICH_LOCAL_DIR` is configured (file-based bootstrap), the local
runner uses its own independent `Always{}` gate so that local bootstrap
runs never contaminate the main runner's Redis gate state.

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

DLQ messages preserve W3C trace context: when the queue routes a task to
the DLQ via `routeDLQ()`, the task's headers (including `h:traceparent`
and `h:tracestate`) are forwarded alongside payload/retry/reason fields.
On `requeue`, any stored `h:*` headers are restored so the trace chain
remains unbroken through DLQ inspection and reprocessing.

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

cmd/                    Process entry points (one main per binary)
  discoverer/  fetcher/  parser/  enricher/
  proxyloader/  migrator/  dlq/

internal/
  bootstrap/            Pure infrastructure helpers (ProxyPool, Postgres, Logger, Telemetry, …)
                        ◆ No god-objects — all helpers are small, focused, and domain-specific.

  dedup/                Seen-set abstraction (inmem + redis) + ErrAlreadySeen
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

  queue/                Pub/Sub/Handler abstractions
    middleware/         OTel trace propagation decorators for Publisher/Subscriber
    redisstreams/       Queue with W3C trace propagation

  resilience/           Circuit breaker, retry policies

  storage/              Ports-and-adapters
    pgclient/           Pool opener + Stores bundle + otelpgx tracer
    matchstore/         MatchWriter/MatchReader interfaces + matchpg adapter
      matchpg/          PG implementation (reads domain.Match from go-core/domain)
      interfaces.go     MatchWriter / MatchReader interfaces
    lookupstore/        LookupReader interface + lookuppg adapter
    partitionstore/     PartitionAdmin interface + partitionpg adapter
    refdatastore/       RefDataWriter interface + refdatapg adapter
    redis/              Connection wrapper + LoadConfig()

  worker/               Pipeline implementations
    discoverer/
    fetcher/
    parser/
      decode.go         Match decoding (outputs go-core/domain.Match)
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

> **Key simplification:** The `internal/config/` package has been deleted.
> Each domain package (queue, payload, redis, proxy, fetcher, parser, discovery)
> exposes its own `LoadConfig()` function. Every `cmd/<name>/main.go` composes
> its own configuration from the exact domain packages it needs, acting as a
> **Composition Root**. There is no central god-config, no shared bootstrap
> wiring, and no hidden dependencies between binaries.

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

Redis Streams propagate W3C trace context via `h:traceparent` / `h:tracestate`
fields. The propagation is handled by **middleware decorators** in
`internal/queue/middleware/telemetry.go`:

- `TracedPublisher` — injects context into `msg.Headers` before `Publish()`
- `TracedSubscriber` — extracts context from `msg.Headers` and creates a child
  span before calling the handler; records stage latency via `LatencySink`

These decorators are composed in each `cmd/<name>/main.go`. The queue itself
(`redisstreams.Queue`) never touches trace headers directly — it treats
both payloads and header fields as opaque `[]byte`/`map[string]string`.

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
via `h:traceparent` and `h:tracestate` fields (the `h:` prefix prevents
collisions with payload fields in the Redis Stream).

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

`internal/bootstrap` provides lightweight, stateless helpers — no god-objects,
no monolithic config. Each `cmd/<binary>` calls only the helpers it needs:

- `ProxyPool(rdb, cfg, log)` — creates a Redis-backed proxy pool
- `Postgres(ctx, cfg, log)` — opens an OTel-traced pgxpool
- `NewLogger(handler)` — wraps a slog.Handler with OTel trace ID injection
- `NewLoggerFromEnv()` — same as `NewLogger` but reads `LOG_LEVEL` from the
  environment (defaults to `INFO`); enables `debug`-level tracing across the
  pipeline without code changes
- `InitTelemetry(ctx, name, endpoint, rate)` — sets up OTLP tracing
- `MatchWriter(db, log)` / `ReferenceWriter(db, log)` — PG store constructors
- `WaitForProxies` / `WaitForPostgres` — block startup until dependencies are reachable

There is no `Core()` function, no shared `Deps` struct, no hidden wiring.
Every binary is a **Composition Root** that explicitly constructs its own
dependency graph from domain-specific configs.

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
**Configuration via environment.** All settings are loaded from env vars into
typed config structs by domain-specific `LoadConfig()` functions. Workers
receive configured instances — no direct `os.Getenv` calls in worker packages.
Each `cmd/<name>/main.go` acts as the **Composition Root**, importing only
the config loaders it needs. No central god-config, no hidden coupling.
**Observability via OpenTelemetry.** All services push traces and metrics via
OTLP to Jaeger. W3C trace context propagates through Redis Streams
for end-to-end visualization.

## Primitives

| Primitive      | Type                | Flows Through                              |
|----------------|---------------------|-------------------------------------------|
| MatchID        | int64               | discoverer → fetch queue → fetcher → parser → matchstore |
| RawPayload     | []byte (JSON)       | fetcher → payload store → parser          |
| Match          | domain.Match        | parser → matchstore.MatchWriter (via go-core/domain) |
| MatchRef       | refdatastore.MatchRef | parser → refdatastore                   |
| Proxy          | proxy.Proxy         | proxyloader → proxy pool → fetcher       |
| HTTPDoer       | worker.HTTPDoer     | fetcher (injected)                     |
| BootstrapMarker| initmarker.BootstrapMarker | enricher → source gating              |
| PatchInfo      | lookupstore.PatchInfo | fetcher/parser → lookupstore.LookupReader |
| QueueTask      | queue.Task          | redisstreams queue + trace context        |

**Type origins:**

- `MatchID` — from upstream API (OpenDota), canonical identifier
- `RawPayload` — JSON blob from `<UPSTREAM>/match/<id>`
- `Match` — `go-core/domain.Match`, the single canonical type across the entire monorepo
- `MatchRef` — reference from refdatastore for hero/item name resolution
- `Proxy` — leased from Redis ZSET, carries endpoint + credentials
- `HTTPDoer` — discovered or injected, used by cycles and fetcher
- `PatchInfo` — resolved from timestamp via `PatchByTimestamp()`

## Failure Handling

| Stage      | Failure                          | Action                                 |
|------------|----------------------------------|----------------------------------------|
| discoverer | Network / timeout                 | Rotate proxy (httpdo), retry up to MaxRetries |
| discoverer | 5xx (upstream error)             | Rotate proxy (httpdo), retry up to MaxRetries |
| discoverer | Non-200 status                   | Wrap as error — outer loop retries indefinitely until 200 or cancelled |
| discoverer | All proxy retries exhausted      | Outer loop retries with exponential backoff (capped at 30s) |
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

Configuration is **fully decentralized**. The monolithic `internal/config/config.go`
has been deleted. Every domain package exposes its own `LoadConfig()` function
reading the corresponding env vars with sensible defaults:

| Package | Function | Prefix |
|---------|----------|--------|
| `internal/storage/redis` | `LoadConfig()` | `REDIS_*` |
| `internal/queue` | `LoadConfig()` | `QUEUE_*` |
| `internal/payload` | `LoadConfig()` | `PAYLOAD_*` |
| `internal/proxy` | `LoadConfig()` | `PROXY_*` |
| `internal/worker/fetcher` | `LoadConfig()` | `FETCHER_*` |
| `internal/worker/parser` | `LoadConfig()` | `PARSER_*` |
| `internal/worker/discovery` | `LoadConfig()` | `DISCOVERY_*` |

Each `cmd/<name>/main.go` imports exactly the config loaders it needs and
builds its own `Config` structs — no shared god-config, no hidden dependencies.
Postgres config still uses `go-core/config.PostgresConfig`. Telemetry config
(`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SAMPLE_RATE`) is read directly from
env vars by each main.go until it gets its own package.

The canonical reference is `deploy/.env` (149 vars, consolidated at the repo
root). Defaults are sensible for single-host development; production overrides
via `.env` or the orchestration layer.

## Extending the System

**Adding a new enrich source.** Implement `enrich.RunSource` in
`internal/enrich/sources/<provider>/`, add it to the sources slice in
`cmd/enricher/main.go`, and declare any dependencies through
`DependsOn()`. The runner's topological sort will schedule it correctly.
Types come from `refdatastore` — the canonical package for reference data.

**Adding a new pipeline stage.** Write a worker under
`internal/worker/<name>/` that implements `worker.Handler` (see
`fetcher` and `parser` for examples), create its config package
(`internal/worker/<name>/config.go`), and wire it in
`cmd/<name>/main.go` as a Composition Root via `worker.Run()`.
Failures should classify into existing `metrics.FailureKind`s where possible.

**Swapping a backend.** Each port has at least an in-memory adapter
useful for tests. To replace Redis Streams with, say, NATS JetStream,
implement `queue.Queue` in a new package and update the bootstrap
helpers — no worker code changes.

**Adding a database column.** Create a new numbered file in
`go-core/schema/migrations/` (`make new-migration NAME=add_column`
from the repo root). The next `migrator` run picks it up automatically.