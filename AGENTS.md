# Repository Guidelines

## Project Structure & Module Organization

This checkout contains a Go application and its product contract:

- `SPECIFICATION.md`: product requirements, action contracts, acceptance scenarios, and unresolved decisions.
- `SDLC.md`: contributor communication and tooling rules.
- `cmd/tk/`: executable entry point; `internal/app/`: domain services; `internal/model/`: persisted types; `internal/store/`: SQL ownership and transactions.
- `internal/server/`: HTTP contracts and embedded `web/` assets. Go tests sit beside their packages.

Place reusable development tools under `tools/`. Keep generated binaries and local credentials out of Git.

Executable tools append UTC start/end times, monotonic durations, outcomes and exit codes to ignored `tool_calls_history.csv`. Python and Go writers share a process lock. Arguments, environment values and exception messages are excluded. `TELEPORT_TOOL_HISTORY` selects an isolated history path for tests. An `exec` launcher records `handoff` before replacing itself; that row does not assert the native process's eventual outcome.

Run `python3 tool/timing.py` to print each recorded invocation and its duration in seconds. Supply a CSV path as the optional positional argument to inspect another history file.

Smoke stage and action-category timings append to ignored `tool_calls_stages.csv`; `run_id` links to the invocation history. `TELEPORT_TOOL_STAGES` selects an isolated stage file. Profiles distinguish database, acceptance slice and browser driver. Stage totals include action times; parallel slices overlap, so summing rows does not give suite wall time. Selectors, arguments and typed values are excluded.

## Requirements & Architecture

Read the authority and change-control rules in `SPECIFICATION.md` before proposing changes. Distinguish requirements, engineering-owned contracts, and open choices. Workflows are directed graphs with configurable outcome rules and backward routes; preserve finite attempts and human approval gates. Seed manual and agentic defaults. The specification alone does not authorize implementation, deployment, or destructive resets.

Preserve the Goal → Epic → Task hierarchy. PostgreSQL is the only application database. Only `tk init` may access the database directly as a client; other application operations use the server API. The ignored `.respec/` directory is an explicitly retained external-tool exception, not application storage.

## Build, Test, and Development Commands

Go 1.27.3+, Python 3 and Make are required to build; Node.js is also required for browser-source development checks. Supported commands:

- `make build`: produce `bin/tk` with build version metadata.
- `make install`: install the executable.
- `make sbom`: generate `sbom.cdx.json` with Go and bundled browser dependencies, licenses and hashes.
- `make browser-tools`: prepare a cached, pinned Python Playwright client and its matching headless Chromium for browser acceptance checks.
- `make deploy`: detect Linux architecture over SSH, build a static target executable and SCP/start it below `/home/exedev/teleport`; strong first-run credentials are written to protected remote/local config files, never command arguments or tool history. Existing installations are never reset.
- `make db-up` / `make db-down`: explicitly start/stop the private local PostgreSQL service without deleting data.
- `go test ./...`: run Go tests with `TELEPORT_TEST_POSTGRES` pointing at an isolated test-only PostgreSQL administration database. `make test` provisions a disposable socket-only cluster automatically.
- `make check`: everyday formatting, vet, documentation, tooling and normal Go tests; no race instrumentation.
- `make check-race` / `make race-check`: explicit full Go race suite, with nice +15, one package at a time and `GOMAXPROCS=2` by default. An explicit `GOMAXPROCS` overrides that default. Windows uses idle process priority.
- `make check-full`: run everyday checks first, then the race suite; the full CI/release validation entry point.
- `python3 -B tools/web_check.py`: check browser syntax, asynchronous-state regressions and bundled Three.js hashes.
- `python3 -B tools/smoke.py --browser`: exercise an isolated real binary/browser installation after building.
- `python3 -B tools/smoke.py --browser --studio-only`: check the Three.js 2D graph, hover/flow, independent layout persistence, Studio menus/tables and workflow Map.
- `python3 -B tools/postgres_check.py --browser`: run the same suite and real browser path against a disposable PostgreSQL cluster; requires local server binaries. Add `--race` to opt into the low-priority race runner.

Run `bin/tk init`, then `bin/tk serve`; neither build nor init starts services. Use the `tk` binary for server operations.

Every `tools/smoke.py` invocation has a 30-second wall-clock budget, including those launched by the PostgreSQL runner. Six seconds are reserved for owned-process/browser cleanup. Expiry exits with status 124 and records outcome `timeout` in the history CSV.

The default PostgreSQL browser suite provisions a disposable socket-only cluster and runs isolated `core`, `shell` and `views` databases concurrently inside that single budget and owned process group. The shell slice retains profile, navigation, keyboard and project checks; core retains ticket delivery, workflows, supervised demonstration and Git; views retains Ideas, Graph, Goals and Calendar. `--browser-slice core|shell|views` selects one slice. `--browser-driver cli` retains the previous command driver for comparisons; `python3 -B tools/browser_benchmark.py` compares identical, successful fill/click/read actions with both drivers. `TELEPORT_POSTGRES_BIN` selects server binaries; Postgres.app and standard Linux installations are discovered automatically.

`make build` always creates a new build identity. `python3 -B tools/build.py --reuse` reuses an artifact only when source and embedded bytes, module files/replacements, toolchain, target, flags, Git identity and binary hash match. PostgreSQL checks use this mode. Workspace/overlay builds disable reuse. `make sbom` caches the Go inventory with a content fingerprint, verifies browser assets and runtime licenses again, and refreshes source identity, timestamp and serial number. `python3 -B tools/sbom.py --refresh` forces inventory regeneration. Remove `.cache/tools` to clear these caches.

## Coding Style & Naming Conventions

Follow `SDLC.md`: use objective or imperative language without first-person pronouns. Prefer Go or Python for development tooling; create reusable tools rather than inline Python scripts, and avoid `curl` where possible.

Use `gofmt`, tab indentation, idiomatic Go package names and `*_test.go` files. Keep HTTP schemas and shipped-action discovery synchronized with services.

## Testing Guidelines

Cover agreed acceptance scenarios with fresh evidence. No numeric coverage threshold is specified. Planned validation includes Go unit, integration, and race tests, browser checks, and both database modes. Routine fixtures must avoid paid model calls; live model acceptance requires separate authorization.

Prefer `make check` during everyday development. Deprioritize full local race runs: use the explicit race target for concurrency/shared-state changes or CI/release validation, and do not repeat it after unrelated documentation or browser-only edits. Ordinary domain fixtures reuse serialized bootstrap data while retaining independent databases, mutable state, installation identities and authenticated sessions. Initialization/authentication tests still exercise real production password hashing; never reduce production hashing cost for test speed. `tools/go_check.py` records invocation timings/outcomes and accepts `--run`, `--count` and a single-package `--profile PATH` for focused diagnosis. Keep profiles/test binaries in ignored `.cache/` or temporary storage.

## Commit & Pull Request Guidelines

Push validated changes to `main` using the phase workflow below. Do not deploy to the hosted site or migrate hosted data unless the user specifically requests that hosted operation. Requests to implement changes, continue work or push to `main` do not authorize deployment. Do not introduce GitHub Actions; keep routine validation local.

Use concise, imperative commit subjects. Each phase starts on its own branch from clean `main`, passes its checks, then merges with `git merge --ff-only`; preserve linear history. Pull requests describe scope, affected requirement/action IDs, validation results and deferred behavior. Include screenshots for visual changes.
