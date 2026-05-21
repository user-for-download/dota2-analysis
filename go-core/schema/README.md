# Schema Migrations

## Rules

1. **Forward-only.** No down-migrations.
2. **Additive by default.** Adding columns, tables, indexes is safe.
3. **Breaking changes require an RFC** and sign-off from both team leads.
4. **Numbering:** strictly sequential (`003_*.sql`, `004_*.sql`, ...).
5. **Naming:** `NNN_short_description.sql`.

## Adding a Migration

```bash
make new-migration NAME=add_foo_column
```

1. Write the SQL in the created file.
2. Update contract tests in `../contracttest/` to assert new invariants.
3. Run locally with a real Postgres:
   ```bash
   POSTGRES_TEST_DSN="postgres://dota2:dota2@localhost:5432/dota2?sslmode=disable" \
     go test ./contracttest/... -v
   ```
4. Open a PR; both teams' CODEOWNERS must approve.

## Production Deployment

Migrations are embedded in the migrator binary at build time. Operators run:

```bash
./migrator -dsn postgres://...
```

## Schema Ownership

| Schema | Owner | Notes |
|--------|-------|-------|
| `public` | Ingestion writes | Matches, players, heroes, etc. |
| `analytics` | Analytics reads/MVs | Materialized views, scoring tables |

Tables in `public` are written by the ingestion pipeline. Tables in `analytics` are populated by analytics featurizers and read by the API.
