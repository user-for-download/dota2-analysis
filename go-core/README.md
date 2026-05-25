# go-dota2-core

Shared module for the Dota 2 analysis monorepo. Provides domain types, bootstrap helpers, schema migrations, and migrator infrastructure used by both `go-ingestion` and `go-analysis`.

## Public API Surface

The following packages are considered public and stable:

| Package | Purpose |
|---------|---------|
| `domain` | Typed IDs (`HeroID`, `MatchID`, `TeamID`, etc.) |
| `bootstrap` | Postgres pool, logger, OTel telemetry init |
| `config` | Shared config structs (Postgres, telemetry) |
| `migrator` | Schema migration runner |
| `schema` | Embedded SQL migrations (`//go:embed`) |
| `contracttest` | Schema invariant and boundary tests |

Packages not listed here are module-private and may change without notice.

## Typed IDs Policy

Typed IDs (`domain.HeroID`, `domain.MatchID`, etc.) should be used at public API boundaries — handler signatures, repository interfaces, and exported package types. Internal helpers may use raw `int64`/`int` for performance or readability when the type information is locally obvious.

### High-value adoption targets

- API handler parameter types and return types
- Repository interface method signatures
- Public constructor functions

## Schema Contract

- **Forward-only migrations.** No down-migrations.
- **Additive by default.** Adding columns, tables, indexes is safe.
- **Breaking changes require sign-off** from both team leads.
- **Numbering:** strictly sequential (`003_launch_keys.sql`, `004_*.sql`, ...).

## Adding a Migration

```bash
# From repo root:
make new-migration NAME=add_foo_column
```

This creates `go-core/schema/migrations/NNN_add_foo_column.sql`.

Then:
1. Write the SQL
2. Update `contracttest/` to assert new invariants
3. Run `go test ./contracttest/...` with a real Postgres
4. Open a PR; both teams' CODEOWNERS must approve

## Module-Boundary Rule

**`go-core` must never import `go-analysis` or `go-ingestion`.** This is enforced by a contract test (`TestCoreHasNoDownstreamImports`). If you need shared functionality, it belongs in `go-core` or a new shared module — never reference downstream from upstream.
