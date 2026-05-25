# Contributing

Thanks for your interest in contributing to the Dota 2 Analysis Pipeline!

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/).
Be respectful, constructive, and inclusive.

## Getting Started

1. Fork the repository.
2. Set up local development (see [README](./README.md#quick-start)):
   - Go 1.26+
   - Docker + Docker Compose
   - PostgreSQL 16 (optional — Docker recommended)
3. Make sure `make test`, `make vet`, and `make build` pass before making changes.

## Module Model

```
go-ingestion ──requires──> go-core <──requires── go-analysis
```

- **`go-core` must never import downstream** (`go-ingestion`, `go-analysis`). This is enforced by `TestCoreHasNoDownstreamImports`.
- `go-core` contains shared domain types, bootstrap, config, and schema migrations.
- `go-ingestion` handles match discovery, fetching, parsing, enrichment, and proxy management.
- `go-analysis` handles feature computation, model scoring, draft recommendations, and backtesting.

Read `go-core/ARCHITECTURE.md` for deeper architectural context.

## Development Workflow

### Branch Naming

```
feat/<short-description>
fix/<short-description>
chore/<short-description>
```

### Before Submitting

1. Run the full test suite: `make test`
2. Run the linter: `make lint` (requires golangci-lint)
3. Run vet: `make vet`
4. Verify Docker images build: `make build-ingestion && make build-analysis`

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(ingestion): add rate-limit retry to fetcher
fix(analysis): correct synergy normalisation
chore(deps): bump pgx to v5.7.2
```

### Pull Request Process

1. Link any related issues.
2. Update the README or `go-core/ARCHITECTURE.md` if public interfaces change.
3. Add or update contract tests when touching schema or module boundaries.
4. Add or update unit tests — new packages must have at least basic coverage.
5. Ensure CI passes (build → test → lint → contract tests → Docker build).
6. Request review from the appropriate team based on the affected module:
   - **Both teams** — changes to `go-core/domain/`, `go-core/schema/`, or `go-core/contracttest/`
   - **Analytics team** — changes under `go-analysis/`
   - **Ingestion team** — changes under `go-ingestion/`
   - **Infra owner** — changes to `deploy/`, `.github/`, `Makefile`, `go.work`

## Coding Standards

- Follow the patterns established in the codebase — typed domain IDs at public boundaries, raw ints internally.
- Use slog for structured logging, not `log.Printf`.
- Use the project's OTel setup (in `go-core/bootstrap/`) for tracing.
- Prefer additive schema migrations — no destructive changes without both-team sign-off.
- Keep `replace` directives in downstream `go.mod` files (go-core is unpublished). Use `go.work` for local development.
- LightGBM model files are versioned with `meta.json` — never commit `.bin` files to git.

## Adding a Migration

```bash
make new-migration NAME=add_foo_column
```

Then write the SQL, update the contract test expectations, and PR with both-team approval.

## Need Help?

Open a [GitHub Issue](https://github.com/user-for-download/dota2-analysis/issues).
