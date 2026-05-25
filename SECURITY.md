# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it privately.

**Do not open a public GitHub issue.** Instead, send details to the project maintainers via email or a private security advisory.

To report privately:

1. Go to [github.com/user-for-download/dota2-analysis/security/advisories](https://github.com/user-for-download/dota2-analysis/security/advisories)
2. Click **"New draft security advisory"**
3. Fill in the details — include a description, affected versions, and reproduction steps

If a private advisory is not possible, reach out to a repository administrator directly.

## What to Expect

- **Acknowledgement** within 48 hours of submitting a report.
- **Initial assessment** within 5 business days — we will determine severity and affected versions.
- **Fix timeline**: critical issues are prioritised and typically addressed within 7 days.
- **Disclosure**: once a fix is released, we will publish a security advisory with details.

## Scope

The following are in scope:

- The Go modules under `go-core/`, `go-analysis/`, `go-ingestion/`
- Docker images built from this repository
- CI/CD configuration under `.github/`

Out of scope:

- Third-party dependencies — report those to their respective maintainers
- The Dota 2 game client or Valve API
- Infrastructure not defined in this repository

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| latest  | :white_check_mark: |
| older   | :x:                |

Only the latest main branch commit is supported. Releases are not tagged — always update to the most recent commit.
