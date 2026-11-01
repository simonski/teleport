# Usage Guide

## Local setup

Run `make build`, then `bin/tk init` and `bin/tk serve`. SQLite defaults to `~/.teleport/database.db`; `--db PATH` or `TELEPORT_DB` selects an isolated installation. The local server defaults to `localhost:9999`; `serve --addr HOST:PORT` overrides it. Repeated init preserves identity and credentials, refuses a running installation, and starts nothing.

Sign in through the browser with initial `admin` / `password`. CLI login reads the password from standard input: `bin/tk login --password-stdin`. `TELEPORT_SERVER` selects the server; `TELEPORT_CONFIG` selects the private mode-0600 client credential file. All CLI operations except init and server hosting use HTTP APIs. `bin/tk logout` revokes that sign-in session.

## Support boundary

Projects creates projects and their Unassigned hierarchy. New creates Goal/Epic/Task tickets; choose Human intent or use Todos for its human default. Click a ticket to edit it or inspect history. Original content changes require Todo; Done asks for explicit completion acceptance. Todos has exactly Todo/Doing/Done; Board defaults to agent intent and offers Human/Both lenses. Home shows actual latest/upcoming work. Keyboard N creates, Q/Shift+Q switches views, and Escape dismisses the current dialog without applying changes. Browser changes poll committed state while preserving open editors.

For example, `bin/tk createProject --data '{"name":"Flight","key":"flight","requestId":"create-flight-1"}'`, then `bin/tk createTicket --data '{"projectId":1,"title":"Pack case","agent":false,"requestId":"pack-1"}'`. Supply the current `expectedVersion` for edits/transitions. `bin/tk listTickets --data '{"projectId":1,"limit":100}'` reads a bounded page. `tk spec` publishes all shipped schemas.

This is an incremental local implementation. Agents and registrations start disabled. Only operations listed by `tk tools` are shipped; unsupported actions fail explicitly. Published human and agent workflow definitions are seeded. Real model execution, Git integration, PTY/remote hosts, multi-user activation, external sign-in, TUI, Graph, automation and release publication are not yet implemented. Private project-draft autosave and broad drag/layout controls remain pending. PostgreSQL has a driver path but requires real parity evidence before support is claimed. Never use demo evidence as proof of real code delivery.

Ticket details offer prerequisites, cancellation and permitted reopen/transfer/archive/delete controls. Visibility changes affect only that record; Settings → Hidden tickets provides restoration. Container cancellation affects unfinished descendants, while container reopening affects only the selection. Transfers require ownership (Maintainer) in both projects and refuse cross-project prerequisite links. Settings → Change password verifies the current password and signs out every session; repeat init never resets it.

Settings → Workflow editor shows published definitions and drafts. New from this workflow creates a project-owned draft; edit phase titles, roles, instructions, agentic settings and outcome rules in the keyboard-accessible lists. Canvas node dragging saves layout only. Save draft before previewing a route or activating it for a selected project. Activation changes future tickets; existing Board sections retain their published version. The CLI also exposes stopped same-role migration, while runtime agent attempts and real execution remain unavailable.

## Development

`make check` verifies documentation, Go formatting/vet and race tests. `make build` increments only the ignored local build counter; checks do not. `TELEPORT_BUILD_VERSION` provides an explicit reproducible build version. `make install` installs the built binary. No command auto-starts a service or browser.
