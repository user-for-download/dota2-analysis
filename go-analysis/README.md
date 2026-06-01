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
2. **Featurizer** refreshes `analytics.*` materialized views (every 24h):
   - Refreshes all MVs → inserts `featurizer_ready` launch key → snapshots
3. **API** blocks at startup on `analytics.launch_keys` key (`WaitForLaunchKey`),
   then reads from MVs, scores candidates, and serves recommendations
4. **Backtester** also blocks on the same key before evaluating models
5. **Trainer** (Python) trains LightGBM models offline

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

- **Linear**: Hand-tuned weighted sum of 17 feature sources. Default, no model files needed.
- **LGBM**: LightGBM LambdaMART model loaded from `assets/models/imitation/current/`. SIGHUP hot-reload via `ModelWatcher` and `ModelReloader` interface.

Switch via `ANALYTICS_SCORER_KIND=linear|lgbm`.

## ML Training

Python package in `training/`. Build with `build/dockerfiles/Dockerfile.trainer`:

```bash
# Extract data → train imitation model → evaluate → publish
trainer all
# Or step by step:
trainer extract
trainer train-imitation
trainer evaluate
trainer publish
```

The training pipeline uses **23 features** across four groups:

| Group | Features | Ranking Signal |
|-------|----------|----------------|
| MV-dependent (0-7) | team_picks, team_wr, synergy, counter, hero_meta, player_comfort, star_threat | Constant across candidates when MVs empty |
| Hero priors (8-10) | pick_rate, wr, popularity | **Primary** — varies per hero |
| Attribute draft (11-14) | attr flags + fit_score | **Secondary** — varies per hero |
| Draft context (15-22) | slot_norm, team/enemy picks before, phase flags | Weak (same across candidates in a group) |

> **Note:** `is_pick_phase` was removed in v2026-06-01 — the training pipeline skips ban-phase candidates entirely, so it was always 1.0. The Go linear scorer still has access to this feature via its own registry.

**Key design decisions:**
- **Match-split before feature computation** — hero priors use training-set-only stats to prevent validation leakage
- **30 negative samples per slot** — keeps group sizes manageable for LambdaMART while providing a realistic ranking pool
- **MV refresh once per pipeline run** — `refresh_mvs=False` on subsequent `compute_features` calls saves minutes
- **Train/eval asymmetry documented** — restricted-set (~31 cand/slot) vs full-pool (~127 cand/slot) metrics are not directly comparable
- **Feature importance saved** to `meta.json` per training run for debugging

**Models produced:**
| Model | Directory | Type | Purpose |
|-------|-----------|------|---------|
| Imitation | `imitation/current/` | LambdaMART ranker | Ranks heroes per draft slot to mimic pro decisions |
| Outcome predictor | `value/current/` | Binary classifier | Predicts P(win \| pick was made) — NOT a pick-value model (see `train_value.py` docstring) |

The pipeline automatically publishes artifacts to `assets/models/imitation/current/` and `assets/models/value/current/` which are hot-reloaded by the API on SIGHUP.

**Training data criteria**: Only professional matches (`leagueid > 0`) in practice or tournament lobbies (`lobby_type IN (1, 2)`) are used for training. This ensures the model learns from competitive match data only.

## Testing

```bash
# Unit tests (no database required)
cd training && pip install ".[test]" && python -m pytest tests/test_features.py -v

# Integration tests (requires Postgres with Dota 2 data)
TRAINER_POSTGRES_DSN=postgresql://dota2:dota2@localhost:5432/dota2 \
  python -m pytest tests/test_integration.py -v -x
```

**Test coverage areas:**
- `test_features.py` — 8 unit tests for leakage prevention, shrinkage math, fillna behaviour, candidate generation edge cases
- `test_integration.py` — 7 integration tests for end-to-end extract → train → evaluate, model artifact validation, NaN/inf guards

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) — detailed deployment runbook and patch transition guide
- `go-core/` — shared domain types, bootstrap, and schema migrations
- `go-ingestion/` — data ingestion pipeline (sibling project)
