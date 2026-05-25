# Enricher Service-Level Agreement (SLA)

> **Service**: Enricher (`go-dota2-enricher`)  
> **Refresh cycle**: Every `ENRICH_INTERVAL` (default: 24h), gated by Redis last-success timestamps.  
> **Data source**: [dotaconstants](https://github.com/odota/dotaconstants) (upstream JSON payloads).

---

## Reference Data Categories

| Source | Critical | Max Stale | Upsert Behavior | Stub Protection |
|---|---|---|---|---|
| Hero catalog | ✅ Yes | ≤ 1 h | INSERT … ON CONFLICT DO UPDATE | `ensure_hero_stubs()` creates temporary stubs; enricher overwrites with real data on next run. |
| Item catalog | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Ability catalog | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Ability-IDs index | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Hero-abilities mapping | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Game modes | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Lobby types | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Regions | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |
| Patches | ❌ No | ≤ 24 h | INSERT … ON CONFLICT DO UPDATE | N/A |

### Stub Hero Alerting

When a match references a hero ID not yet in the `heroes` table, the `trg_ensure_hero_stub` trigger creates a placeholder row (`unknown_{id}`). The enricher resolves these stubs when it next fetches the hero catalog.

**Monitoring query**:
```sql
SELECT * FROM check_stale_stubs();        -- default 1 h threshold
SELECT * FROM check_stale_stubs('30m');   -- custom threshold
```

**Alert threshold**: `hero_id != 0` in the result → stale stubs exist beyond the allowed window.

---

## Assessment Criteria

### 1. Hero Catalog Freshness
- **SLA**: The heroes table should contain no stub entries older than 1 hour.
- **Measurement**: `SELECT * FROM check_stale_stubs()`
- **Enforcement**: `trg_ensure_hero_stub` trigger on `draft_timings`, `picks_bans`, `player_matches` creates stubs on insert.
- **Recovery**: If the enricher is lagging (`ENRICH_INTERVAL` too long or network failures), set `ENRICH_FORCE_BOOTSTRAP=true` and restart to bypass the Redis gate.

### 2. Non-Critical Reference Data
- **SLA**: All non-critical data should be refreshed within one enricher cycle (≤24 h).
- **Measurement**: Compare `updated_at` in reference tables against `NOW() - ENRICH_INTERVAL`.
- **Enforcement**: The enricher gate in Redis prevents redundant fetches within `ENRICH_INTERVAL`.

### 3. Enricher Gate Behavior
- **Redis keys**: `{ENRICH_BOOTSTRAP_PREFIX}:{source_name}` (default: `dota2:enrich:dotaconstants:heroes`).
- **TTL**: Same as `ENRICH_INTERVAL` (default 24 h). When the key expires, the next run will re-fetch.
- **Force mode**: `ENRICH_FORCE_BOOTSTRAP=true` bypasses the gate — every source runs unconditionally on next start.

---

## Configuration Reference

| Variable | Default | Description |
|---|---|---|
| `ENRICH_INTERVAL` | `24h` | Minimum time between successive enricher runs for any given source. Also used as the Redis gate TTL. |
| `ENRICH_RUN_AT_START` | `true` | If true, runs all sources immediately on startup. |
| `ENRICH_FORCE_BOOTSTRAP` | `false` | Bypasses the Redis gate — every source runs unconditionally. |
| `ENRICH_HTTP_TIMEOUT` | `30s` | HTTP client timeout for dotaconstants fetches. |
| `ENRICH_WAIT_TIMEOUT` | `5m` | How long the enricher waits for upstream connectivity before failing. |
| `ENRICH_MAX_PROXY_RETRIES` | `5` | Max retries per HTTP request through the proxy pool. |

---

## Default Partition Alerting

The schema includes a monitoring function for time-range default partitions (catch-all partitions that should remain empty under normal operation):

```sql
-- Check all default partitions
SELECT * FROM check_default_partitions();           -- default 100-row threshold
SELECT * FROM check_default_partitions(50);         -- custom threshold
```

**Alert levels**:
- `WARNING`: rows > threshold (default 100) — possible missing partition for current quarter.
- `CRITICAL`: rows > 10× threshold — partition creation has been broken for multiple quarters.

**Recovery**: Run `SELECT ensure_future_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], 4)` to create missing partitions, then `SELECT drop_old_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], 2)` to clean data after partitions are created.

---

## Partition Retention SLA

- **Retention window**: All quarterly time-range partitions older than 2 years should be dropped.
- **Enforcement**: `SELECT * FROM drop_old_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], 2)`
- **Scheduling**: This can be run via pg_cron (if installed) or an external cron job every month.
- **What gets dropped**: Partitions matching the pattern `{table}_{year}_q{quarter}` where year < current year − retention_years.

---

## Monitoring Dashboards

Key views for operational monitoring:
1. `v_default_partition_health` — Row count per default partition.
2. `v_unknown_heroes` — Current stub heroes and their age.
3. `check_default_partitions(threshold)` — Alert-level summary of default partitions.
4. `check_stale_stubs(max_age)` — Stub heroes exceeding freshness threshold.
