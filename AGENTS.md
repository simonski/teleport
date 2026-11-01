# Repository Guidelines

## Project Structure & Module Organization

This checkout contains a Go application and its product contract:

- `SPECIFICATION.md`: product requirements, action contracts, acceptance scenarios, and unresolved decisions.
- `SDLC.md`: contributor communication and tooling rules.
- `IMPLEMENTATION_PLAN.md`: phase branches, dependencies and verification evidence.
- `cmd/tk/`: executable entry point; `internal/app/`: domain services; `internal/model/`: persisted types; `internal/store/`: SQL ownership and transactions.
- `internal/server/`: HTTP contracts and embedded `web/` assets. Go tests sit beside their packages.

Place reusable development tools under `tools/`. Keep generated binaries and local credentials out of Git.

## Requirements & Architecture

Read the authority and change-control rules in `SPECIFICATION.md` before proposing changes. Distinguish requirements, engineering-owned contracts, and open choices. Workflows are directed graphs with configurable outcome rules and backward routes; preserve finite attempts and human approval gates. Seed manual and agentic defaults. The specification alone does not authorize implementation, deployment, or destructive resets.

Preserve the Goal → Epic → Task hierarchy. SQLite is the default database; PostgreSQL is also required. Only `tk init` may access the database directly as a client; other application operations use the server API.

## Build, Test, and Development Commands

Go 1.26.1+, Python 3 and Make are required. Supported commands:

- `make build`: produce `bin/tk` with build version metadata.
- `make install`: install the executable.
- `go test ./...`: run Go tests.
- `go test -race ./...`: check Go code for data races.
- `make check`: verify formatting, vet, documentation and race tests.
- `python3 -B tools/smoke.py --browser`: exercise an isolated real binary/browser installation after building.
- `python3 -B tools/postgres_check.py --browser`: run the same suite and real browser path against a disposable PostgreSQL cluster; requires local server binaries.

Run `bin/tk init`, then `bin/tk serve`; neither build nor init starts services. Use the `tk` binary for server operations. See `USER_GUIDE.md` and `API_CONTRACT.md` for shipped behavior.

## Coding Style & Naming Conventions

Follow `SDLC.md`: use objective or imperative language without first-person pronouns. Prefer Go or Python for development tooling; create reusable tools rather than inline Python scripts, and avoid `curl` where possible.

Use `gofmt`, tab indentation, idiomatic Go package names and `*_test.go` files. Keep HTTP schemas and shipped-action discovery synchronized with services.

## Testing Guidelines

Cover agreed acceptance scenarios with fresh evidence. No numeric coverage threshold is specified. Planned validation includes Go unit, integration, and race tests, browser checks, and both database modes. Routine fixtures must avoid paid model calls; live model acceptance requires separate authorization.

## Commit & Pull Request Guidelines

Use concise, imperative commit subjects. Each phase starts on its own branch from clean `main`, passes its checks, then merges with `git merge --ff-only`; preserve linear history. Pull requests describe scope, affected requirement/action IDs, validation results and deferred behavior. Include screenshots for visual changes.
