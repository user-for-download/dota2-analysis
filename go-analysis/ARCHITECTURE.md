# go-dota2-analysis — Architecture & Operations

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Patch Transition Runbook](#patch-transition-runbook)
- [Deployment Guide](#deployment-guide)
- [API Documentation](#api-documentation)
- [Model Update Procedure](#model-update-procedure)
- [Troubleshooting Guide](#troubleshooting-guide)

---

## Architecture Overview

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        Docker Network: dota2-net                │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌───────────────────┐  │
│  │  Ingestion   │    │   Postgres   │    │   Redis           │  │
│  │  (sibling)   │───▶│   16-alpine  │◀───│   7-alpine        │  │
│  │  fetcher,    │    │   :5432      │    │   :6379           │  │
│  │  parser,     │    │              │    │                   │  │
│  │  enricher    │    │  public.*    │    └───────────────────┘  │
│  └──────────────┘    │  analytics.* │                           │
│                      └──────┬───────┘                           │
│                             │                                   │
│              ┌──────────────┼──────────────┐                    │
│              │              │              │                    │
│              ▼              ▼              ▼                    │
│  ┌──────────────────┐  ┌──────────────┐ ┌───────────────────┐ │
│  │   Featurizer     │  │    API       │ │   Trainer         │ │
│  │  (periodic)      │  │  (HTTP :8080)│ │  (on-demand)      │ │
│  │                  │  │              │ │  profiles:training│ │
│  │ 1. RefreshAll MVs│  │ Waits for    │ │                   │ │
│  │ 2. Set launch_key│──│ featurizer   │ │ Trains LGBM       │ │
│  │ 3. Snapshot      │  │  ready key   │ │                   │ │
│  └────────┬─────────┘  └──────────────┘ └───────────────────┘ │
│           │ launch_key                                         │
│           ▼                                                    │
│  ┌──────────────────┐                                          │
│  │  analytics.launch│                                          │
│  │  _keys table     │  ┌──────────────┐                        │
│  │  (Postgres)      │  │  Backtester  │  (on-demand, offline)  │
│  │                  │──│  (waits for  │  Replays historical    │
│  └──────────────────┘  │   key too)   │  drafts                │
│                        └──────────────┘                        │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Ingestion** (sibling project) writes raw match data to `public.*` tables
2. **Featurizer** runs on a schedule (default: every 24h):
   - Refreshes materialized views in `analytics.*` (hero profiles, synergies, counters)
   - Inserts `featurizer_ready` key into `analytics.launch_keys` (unblocks API/backtester)
   - Takes point-in-time snapshots to `analytics.feature_snapshots_player_hero`
   - Updates `analytics.featurizer_runs` with execution metadata
3. **API** blocks at startup via `bootstrap.WaitForLaunchKey()` until the featurizer
   has populated the MVs, then serves HTTP requests:
   - Reads from `analytics.*` MVs and `public.*` tables (read-only)
   - Uses linear scorer (hand-tuned weights) or LGBM model for draft recommendations
   - LGBM model loaded from `/models/imitation/current/` with SIGHUP hot-reload
   - Backtester also blocks on the same key before evaluating models
4. **Trainer** (Python, on-demand):
   - Reads historical data from Postgres
   - Trains LightGBM model → writes `model.bin`, `spec.json`, `meta.json` to `/models/`
5. **Backtester** (offline):
   - Replays historical drafts against the current model
   - Reports baseline metrics (accuracy, pick rate correlation)

### Key Design Principles

- **Read-side only**: Analysis service never writes to `public.*`
- **All derived data in `analytics.*` schema**
- **Python trains, Go infers**: The seam is the model directory
- **Personal use**: No Redis cache, no HA, no rate limiting

---

## Patch Transition Runbook

### When a New Dota 2 Patch Drops

The analysis service uses a patch-window filter (`patch_id >= MAX(patch_id) - 2`) in all materialized views. When Valve releases a new patch, the MVs must be rebuilt to reflect the new meta.

### Step 1: Update Patch ID

Update `ANALYTICS_PATCH_ID` in all service environments:

```bash
# Edit your .env file
export ANALYTICS_PATCH_ID=<new_patch_id>

# Or set it directly in .env:
# ANALYTICS_PATCH_ID=73
```

### Step 2: Rebuild Materialized Views

Recreate (not refresh) MVs that have a patch-window filter. The `MAX(patch_id)` subquery will automatically pick up the new patch, but you must rebuild to populate data:

```sql
-- Drop and recreate all analytics MVs
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_team_hero_profile    CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_hero_synergy         CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_hero_counter         CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_player_team_history  CASCADE;
DROP MATERIALIZED VIEW IF EXISTS analytics.mv_player_hero_profile  CASCADE;

-- Recreate using the migration SQL (run 002_analytics.sql)
-- Or use the migration runner:
-- docker compose --profile all up migrator
```

Alternatively, write a migration `003_rebuild_analytics.sql` that drops and recreates all MVs with the new patch range.

### Step 3: Run Featurizer Immediately

```bash
docker compose --profile all up featurizer
```

This will:
- Refresh all materialized views with the new patch data
- Take a fresh snapshot of player-hero profiles
- Update `analytics.featurizer_runs` with the new timestamp

### Step 4: Restart API

```bash
docker compose restart api
```

This ensures the API picks up the new `ANALYTICS_PATCH_ID` environment variable.

### Step 5: Retrain Model

Wait 2-4 weeks for enough games on the new patch, then:

```bash
docker compose --profile training up trainer
```

The trainer will:
- Read historical data filtered by the new patch window
- Train a new LightGBM model
- Write artifacts to `/models/imitation/current/`

### Step 6: Verify

- Check `/v1/health` for featurizer staleness:
  ```bash
  curl -H "Authorization: Bearer $API_TOKEN" http://localhost:8080/v1/health
  ```
  Verify `featurizer_staleness_hours` is low and `status` is `"ok"`.

- Run backtester to verify baseline metrics:
  ```bash
  docker compose --profile all up backtester
  ```

- Compare LGBM model performance against linear scorer by checking recommendation quality.

### Verification Scripts

#### Derive Phase Table

Extract the empirical draft phase table from the current patch:

```bash
psql -f sql/derive_phase_table.sql
```

#### Verify Phase Table

Validate the phase table against known Captain's Mode patterns:

```bash
psql -f sql/verify_phase_table.sql


### Rollback

If something goes wrong:

1. Revert `ANALYTICS_PATCH_ID` to previous value in `.env`
2. Restart API: `docker compose restart api`
3. Rebuild MVs with old patch range (re-run migration or manual SQL)

---

## Deployment Guide

### Prerequisites

- Docker Compose v2+
- Postgres 16 (provided by sibling project's compose)
- Go 1.26.2 (for local builds)
- Python 3.10+ with lightgbm, pandas, scikit-learn (for trainer)

### Quick Start

The analysis services are part of the monorepo root `deploy/docker-compose.yml`. No separate include is needed — analysis services are grouped under the `analytics` profile alongside the sibling ingestion project's `ingestion` profile.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `ANALYTICS_PATCH_ID` | `72` | Current Dota 2 patch ID for filtering |
| `ANALYTICS_SCORER_KIND` | `linear` | Scorer type: `linear` or `lgbm` |
| `ANALYTICS_MODEL_DIR` | `/models/imitation/current` | Path to LGBM model directory |
| `ANALYTICS_FEATURIZER_INTERVAL` | `24h` | Featurizer refresh interval |
| `API_TOKEN` | `changeme` | Bearer token for API authentication |
| `API_BIND` | `0.0.0.0:8080` | API bind address |
| `ANALYSIS_API_PORT` | `8080` | Host port mapping |
| `POSTGRES_DSN` | (see below) | Postgres connection string |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (empty) | OTel collector endpoint (optional) |

### Docker Compose Profiles

| Profile | Services |
|---|---|
| `all` | api, featurizer, backtester |
| `training` | trainer (one-shot) |

### Migration Steps

Migrations are run via the canonical migrator at `go-core/schema/migrations/`.

1. Verify migration:
   ```sql
   SELECT version, filename FROM public.schema_migrations ORDER BY version;
   -- Should show: 1 (init), 2 (analytics), 3 (launch_keys)
   ```

2. Run featurizer to populate MVs (sets `featurizer_ready` launch key):
   ```bash
   docker compose --profile all up featurizer
   ```

3. API and backtester now auto-wait for the launch key — no manual sequencing needed.

### Building Images

#### Using Docker Buildx Bake (Recommended)

All images share a common base stage for efficient parallel builds:

```bash
# Build all images (api, featurizer, backtester, migrator)
docker buildx bake -f deploy/docker-bake.hcl --load

# Build with a custom tag
docker buildx bake -f deploy/docker-bake.hcl --set *.args.TAG=v1.0.0 --load

# Build a single target
docker buildx bake -f deploy/docker-bake.hcl api --load

# Push to a registry
docker buildx bake -f deploy/docker-bake.hcl --push
```

**Image naming convention:**
- `go-dota2-analysis-base:${TAG}` — shared builder stage
- `go-dota2-analysis-api:${TAG}` — API server
- `go-dota2-analysis-featurizer:${TAG}` — featurizer worker
- `go-dota2-analysis-backtester:${TAG}` — backtester CLI

### Directory Structure (go-analysis module)

```
go-analysis/
├── ARCHITECTURE.md              # This file
├── assets/
│   └── models/                  # ML model artifacts
│       ├── imitation/current/   # Active imitation learning model
│       └── value/               # Value model directory
├── build/
│   └── dockerfiles/             # Docker build definitions
│       ├── Dockerfile.base      # Multi-arch Go builder (all binaries)
│       ├── Dockerfile.featurizer# Featurizer runtime image
│       ├── Dockerfile.backtester# Backtester runtime image
│       └── Dockerfile.trainer   # Trainer runtime image
├── cmd/                         # Entry points (one main per binary)
│   ├── api/
│   ├── backtester/
│   ├── featurizer/
│   └── migrator/
├── internal/                    # Domain logic (11 packages)
├── sql/                         # Operational SQL scripts (derive/verify phase table)
├── training/                    # Python ML trainer (LightGBM)
├── go.mod / go.sum
```

Orchestration files live at the monorepo root:
```
dota2-analysis/
├── deploy/                      # Canonical orchestration
│   ├── docker-compose.yml       # Full stack composition
│   ├── docker-bake.hcl          # Docker Buildx bake definitions
│   └── compose/                 # Compose overrides
└── go-core/schema/migrations/   # Versioned SQL migrations (001_*, 002_*, 003_*)
```

---

## API Documentation

Base URL: `http://localhost:8080`

All API routes require a Bearer token:
```
Authorization: Bearer <API_TOKEN>
```

### Health

```
GET /v1/health
```

Returns service health including featurizer staleness.

**Response:**
```json
{
  "status": "ok",
  "patch_id": 72,
  "scorer": "linear",
  "model_version": "",
  "last_featurizer_success": "2026-05-20T12:00:00Z",
  "featurizer_staleness_hours": 2.5
}
```

**Status values:**
- `"ok"` — featurizer ran within the last 36 hours
- `"stale"` — featurizer has not run in over 36 hours or never ran

---

### Draft Recommendation

```
POST /v1/recommend
```

Get draft recommendations for the current draft state.

**Request:**
```json
{
  "patch_id": 72,
  "user_team": "radiant",
  "radiant_team_id": 8255888,
  "dire_team_id": 8261500,
  "radiant_roster": [86745912, 94155156],
  "dire_roster": [101495620, 19672350],
  "radiant_picks": [1],
  "dire_picks": [2],
  "radiant_bans": [10, 11],
  "dire_bans": [20, 21],
  "slot": 3,
  "k": 10
}
```

**Response:**
```json
{
  "phase": "ban_phase_2",
  "is_ban": true,
  "acting_team": "radiant",
  "recommendations": [
    {
      "hero_id": 5,
      "name": "Crystal Maiden",
      "score": 0.85,
      "rank": 1,
      "reasons": ["synergy: strong with Anti-Mage (0.72 WR)"],
      "risks": ["low sample: only 3 games"]
    }
  ]
}
```

---

### Draft Simulation

```
POST /v1/draft/simulate
```

Simulate a full draft, getting recommendations at each pick slot.

**Request:**
```json
{
  "patch_id": 72,
  "radiant_team_id": 8255888,
  "dire_team_id": 8261500,
  "radiant_roster": [86745912, 94155156],
  "dire_roster": [101495620, 19672350],
  "k": 5
}
```

**Response:**
```json
{
  "steps": [
    {
      "slot": 1,
      "phase": "ban_phase_1",
      "is_ban": true,
      "acting_team": "dire",
      "recommendations": [...]
    }
  ]
}
```

---

### Team Profile

```
GET /v1/teams/{id}/profile
```

Get a team's hero pick history and win rates.

**Response:**
```json
{
  "team_id": 8255888,
  "name": "Team Spirit",
  "hero_history": [
    {
      "hero_id": 1,
      "hero_name": "Anti-Mage",
      "games": 15,
      "wins": 10,
      "wr_shrunk": 0.62
    }
  ]
}
```

---

### Head-to-Head

```
GET /v1/h2h?team_a=X&team_b=Y
```

Get head-to-head record between two teams.

**Response:**
```json
{
  "team_a": 8255888,
  "team_b": 8261500,
  "games": 12,
  "team_a_wins": 7,
  "team_b_wins": 5
}
```

---

### Hero Synergy

```
GET /v1/heroes/{id}/synergy
```

Get heroes that synergize well with the given hero (same team).

**Response:**
```json
{
  "hero_id": 1,
  "hero_name": "Anti-Mage",
  "partners": [
    {
      "hero_id": 5,
      "hero_name": "Crystal Maiden",
      "games": 8,
      "wr_shrunk": 0.72
    }
  ]
}
```

---

### Hero Counter

```
GET /v1/heroes/{id}/counter
```

Get heroes that counter the given hero (opposing team).

**Response:**
```json
{
  "hero_id": 1,
  "hero_name": "Anti-Mage",
  "counters": [
    {
      "hero_id": 35,
      "hero_name": "Bloodseeker",
      "games": 12,
      "wr_shrunk": 0.35
    }
  ]
}
```

---

### Player Profile

```
GET /v1/players/{account_id}/profile
```

Get a player's hero comfort and recent team history.

**Response:**
```json
{
  "account_id": 86745912,
  "hero_history": [
    {
      "hero_id": 1,
      "hero_name": "Anti-Mage",
      "games": 20,
      "wins": 14,
      "wr_shrunk": 0.68,
      "last_played": 1716220800
    }
  ],
  "recent_teams": [
    {
      "team_id": 8255888,
      "games": 45,
      "last_played": 1716220800,
      "is_recent": true
    }
  ]
}
```

---

## Model Update Procedure

### Deploying a New LGBM Model

1. **Train the model** (after 2-4 weeks of new patch data):
   ```bash
   docker compose --profile training up trainer
   ```

2. **Verify artifacts** were produced:
   ```bash
   ls -la assets/models/imitation/current/
   # Should contain: model.bin, spec.json, meta.json
   ```

3. **Switch scorer to LGBM** (if currently using linear):
   ```bash
   export ANALYTICS_SCORER_KIND=lgbm
   docker compose restart api
   ```

4. **Verify model loaded**:
   ```bash
   curl -H "Authorization: Bearer $API_TOKEN" http://localhost:8080/v1/health
   # Check "scorer": "lgbm" and "model_version" is populated
   ```

### Hot-Reload (SIGHUP)

If the API is already running with LGBM scorer, you can hot-reload the model without restarting:

```bash
# Copy new model files to the model directory
cp /path/to/new/model.bin assets/models/imitation/current/
cp /path/to/new/spec.json assets/models/imitation/current/
cp /path/to/new/meta.json assets/models/imitation/current/

# Send SIGHUP to trigger reload
docker kill --signal=SIGHUP dota2-analysis-api
```

The API will atomically swap the model pointer — zero downtime.

### Rolling Back to Linear Scorer

```bash
export ANALYTICS_SCORER_KIND=linear
docker compose restart api
```

---

## Troubleshooting Guide

### Common Issues

#### Featurizer Staleness

**Symptom:** `/v1/health` returns `"status": "stale"`

**Causes:**
- Featurizer has not run in over 36 hours
- Featurizer crashed on last run
- No games in the current patch window yet

**Fix:**
```bash
# Run featurizer manually
docker compose --profile all up featurizer

# Check featurizer logs
docker logs dota2-analysis-featurizer

# Check featurizer_runs table
psql -c "SELECT * FROM analytics.featurizer_runs;"
```

#### Empty Recommendations

**Symptom:** `POST /v1/recommend` returns empty recommendations array

**Causes:**
- Materialized views are empty (not populated after migration)
- Patch ID filter excludes all data
- No heroes match the draft constraints

**Fix:**
```bash
# Check MV row counts
psql -c "SELECT 'mv_team_hero_profile' as mv, COUNT(*) FROM analytics.mv_team_hero_profile
         UNION ALL SELECT 'mv_hero_synergy', COUNT(*) FROM analytics.mv_hero_synergy
         UNION ALL SELECT 'mv_hero_counter', COUNT(*) FROM analytics.mv_hero_counter;"

# Run featurizer to populate
docker compose --profile all up featurizer

# Verify patch ID
psql -c "SELECT MAX(patch_id) FROM public.matches;"
```

#### Model Not Loading

**Symptom:** `/v1/health` shows `"scorer": "lgbm"` but `"model_version": ""`

**Causes:**
- Model files missing from `/models/imitation/current/`
- `spec.json` does not match expected feature spec
- `model.bin` is corrupted

**Fix:**
```bash
# Check model files exist
ls -la assets/models/imitation/current/

# Check API logs for load errors
docker logs dota2-analysis-api

# Revert to linear scorer
export ANALYTICS_SCORER_KIND=linear
docker compose restart api
```

#### Authentication Errors

**Symptom:** `401 Unauthorized` on all API requests

**Causes:**
- Missing `Authorization` header
- Invalid Bearer token
- Token mismatch with `API_TOKEN` env var

**Fix:**
```bash
# Verify token
echo $API_TOKEN

# Test with correct token
curl -H "Authorization: Bearer $API_TOKEN" http://localhost:8080/v1/health
```

#### Postgres Connection Failed

**Symptom:** Services fail to start with Postgres connection errors

**Causes:**
- Postgres not running or not healthy
- Wrong `POSTGRES_DSN`
- Network issue between containers

**Fix:**
```bash
# Check Postgres health
docker compose ps postgres

# Check Postgres logs
docker logs dota2-postgres

# Verify network
docker network inspect dota2-net
```

#### Phase Table Validation Fails

**Symptom:** `verify_phase_table.sql` returns FAIL checks

**Causes:**
- Insufficient data in the current patch
- Data corruption in `picks_bans` table
- Non-standard draft format in some matches

**Fix:**
```bash
# Derive and inspect the phase table
psql -f sql/derive_phase_table.sql

# Check raw data quality
psql -c "SELECT patch_id, COUNT(*) FROM matches GROUP BY patch_id ORDER BY patch_id DESC LIMIT 5;"
```

### Useful Diagnostic Queries

```sql
-- Check MV freshness
SELECT last_mv_refresh_at, mv_refresh_status, patch_min, patch_max, mv_rows_total
FROM analytics.featurizer_runs;

-- Check patch distribution
SELECT patch_id, COUNT(*) as games
FROM public.matches
GROUP BY patch_id
ORDER BY patch_id DESC
LIMIT 10;

-- Check MV row counts
SELECT 'team_hero_profile' as mv, COUNT(*) FROM analytics.mv_team_hero_profile
UNION ALL SELECT 'hero_synergy', COUNT(*) FROM analytics.mv_hero_synergy
UNION ALL SELECT 'hero_counter', COUNT(*) FROM analytics.mv_hero_counter
UNION ALL SELECT 'player_team_history', COUNT(*) FROM analytics.mv_player_team_history
UNION ALL SELECT 'player_hero_profile', COUNT(*) FROM analytics.mv_player_hero_profile;

-- Check launch key state (service startup synchronization)
SELECT * FROM analytics.launch_keys;

-- Check feature snapshot history
SELECT snapshot_at, COUNT(*) as rows
FROM analytics.feature_snapshots_player_hero
GROUP BY snapshot_at
ORDER BY snapshot_at DESC
LIMIT 10;
```

### Log Locations

```bash
# API logs
docker logs dota2-analysis-api

# Featurizer logs
docker logs dota2-analysis-featurizer

# Trainer logs
docker logs dota2-analysis-trainer

# Backtester logs
docker logs dota2-analysis-backtester

# Follow logs in real-time
docker logs -f dota2-analysis-api
```
