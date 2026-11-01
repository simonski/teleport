# Repository Guidelines

## Project Structure & Module Organization

This checkout contains planning documents and documentation tools:

- `SPECIFICATION.md`: product requirements, action contracts, acceptance scenarios, and unresolved decisions.
- `SDLC.md`: contributor communication and tooling rules.

No application source, tests, assets, dependency manifests, or build configuration exist yet. Verify specified paths before using them. Place reusable development tools under `tools/`.

## Requirements & Architecture

Read the authority and change-control rules in `SPECIFICATION.md` before proposing changes. Distinguish requirements, engineering-owned contracts, and open choices. Workflows are directed graphs with configurable outcome rules and backward routes; preserve finite attempts and human approval gates. Seed manual and agentic defaults. The specification alone does not authorize implementation, deployment, or destructive resets.

Preserve the Goal → Epic → Task hierarchy. SQLite is the default database; PostgreSQL is also required. Only `tk init` may access the database directly as a client; other application operations use the server API.

## Build, Test, and Development Commands

Run `python3 tools/spec_sources.py --verify` for documentation checks. Application build, test, and server commands are not executable yet. The specification targets:

- `make build`: produce `bin/tk` with build version metadata.
- `make install`: install the executable.
- `go test ./...`: run Go tests once a Go module exists.
- `go test -race ./...`: check Go code for data races.

Document actual prerequisites and startup commands when implementing the build system. Use the `tk` binary for server operations.

## Coding Style & Naming Conventions

Follow `SDLC.md`: use objective or imperative language without first-person pronouns. Prefer Go or Python for development tooling; create reusable tools rather than inline Python scripts, and avoid `curl` where possible.

For new Go code, use `gofmt` formatting, including tab indentation, idiomatic package names, and `*_test.go` test files. No formatter or lint configuration currently exists.

## Testing Guidelines

Cover agreed acceptance scenarios with fresh evidence. No numeric coverage threshold is specified. Planned validation includes Go unit, integration, and race tests, browser checks, and both database modes. Routine fixtures must avoid paid model calls; live model acceptance requires separate authorization.

## Commit & Pull Request Guidelines

No Git metadata or commit history is available here, so no existing message convention can be established. Use concise, imperative commit subjects. Pull requests should describe scope, affected requirement/action IDs, validation results, and deferred behavior. Include screenshots for visual changes.
