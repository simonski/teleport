# Usage Guide

## Local setup

Run `make build`, then `bin/tk init` and `bin/tk serve`. SQLite defaults to `~/.teleport/database.db`; `--db PATH` or `TELEPORT_DB` selects an isolated installation. The local server defaults to `localhost:9999`; `serve --addr HOST:PORT` overrides it. Repeated init preserves identity and credentials, refuses a running installation, and starts nothing.

Sign in through the browser with initial `admin` / `password`. CLI login reads the password from standard input: `bin/tk login --password-stdin`. `TELEPORT_SERVER` selects the server; `TELEPORT_CONFIG` selects the private mode-0600 client credential file. All CLI operations except init and server hosting use HTTP APIs. `bin/tk logout` revokes that sign-in session.

## Support boundary

This is an incremental local implementation. Agents and registrations start disabled. Only operations listed by `tk tools` are shipped; unsupported actions fail explicitly. Published human and agent workflow definitions are seeded. Real model execution, Git integration, PTY/remote hosts, multi-user activation, external sign-in, TUI, Graph, automation and release publication are not yet implemented. PostgreSQL has a driver path but requires real parity evidence before support is claimed. Never use demo evidence as proof of real code delivery.

## Development

`make check` verifies documentation, Go formatting/vet and race tests. `make build` increments only the ignored local build counter; checks do not. `TELEPORT_BUILD_VERSION` provides an explicit reproducible build version. `make install` installs the built binary. No command auto-starts a service or browser.
