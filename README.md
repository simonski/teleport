# teleport

A local workspace for human tickets and supervised agent workflows. Go 1.27.3+, Python 3 and Make are required to build; PostgreSQL 16+ server binaries are required for local setup and checks.

```sh
make build
make db-up
./bin/tk init
./bin/tk serve
```

Open http://localhost:9999 and sign in with the initial local account `admin` / `password`. Initialization starts no listener or agent.

PostgreSQL is the sole application database. `make db-up` explicitly starts a private socket-only development cluster and writes owner-only `~/.teleport/database.json`; `make db-down` stops it without deleting data. An existing installation requires explicit migration before local setup. For an external instance, set `TELEPORT_DB` or provide `-db-config PATH` containing `{"url":"postgresql://…"}` with mode `0600`. Keep passwords out of command arguments. Test and smoke tools provision separate disposable clusters automatically; `TELEPORT_POSTGRES_BIN` selects server binaries.

The browser bundles Three.js 0.187.0 under its [MIT license](internal/server/web/vendor/LICENSE). `python3 -B tools/vendor_web.py --verify` checks the pinned artifact hashes offline. No CDN or Node runtime is required by the built application.

Run `make sbom` to generate `sbom.cdx.json`.

Release builds use size-optimized C code, stripped native debugging information and a compressed embedded web bundle. `make build-debug` retains native debug information when needed. `python3 -B tools/pack_web.py` refreshes the committed bundle after editing browser assets; normal builds refresh it automatically, and `--verify` detects a stale bundle. Deployments use compressed SCP while retaining binary hash verification.

`make check` is the lightweight source/unit gate: formatting, Python/browser syntax, serial browser unit tests, documentation and tooling tests. It starts no application Go suite, database, real browser or deliberate timeout fixture. Preview its exact scope with `python3 -B tools/check.py --plan`. Broad checks are currently paused; use focused tests until explicitly resumed.

Use `make check-go` (or `make test`) for normal Go tests with disposable PostgreSQL, `make check-integration` for full vet plus Go/database, cross-language history, browser-driver and timeout/cleanup checks, and `make race-check` (also `make check-race`) for explicit race instrumentation. Go checks use low scheduling priority, two execution threads by default and one package at a time; an explicit `GOMAXPROCS` is preserved. `make check-full` runs lightweight, integration and race checks in sequence for a deliberate local/release gate. No GitHub Actions are configured.

The captured [product roadmap](planning/roadmap.json) separates MVP and later stories and retains unresolved model questions. `python3 -B tools/capture_backlog.py planning/roadmap.json --plan` previews its areas without API writes; authorized capture uses the existing signed-in `tk` identity, stable request IDs and inactive tickets with execution off.

Run `tk skill > SKILL.md` to export the bundled agent skill for a harness. No running server or sign-in is required for this export.

Bare `tk` or `tk help` lists the top-level commands. Use `tk agent` (or `tk help agent`) to list that command's actions, and `tk agent ls -h` for typed flags. `tk commands` prints the full catalogue; add `-json` for machine-readable output. Examples: `tk project ls`, `tk ticket show 35`, `tk agent ls -project 2`, `tk idea ls`, and `tk workflow ls`. Agent listing reads accessible harness sessions; starting one remains explicit. Existing API-style commands such as `tk listTickets -data '{"projectId":2,"limit":50}'` keep working.

`tk project ls` lists accessible projects. A signed-in administrator can run `tk project rm NAME`; the command previews affected records and retained artifacts, then requires the exact project name as confirmation. Active execution and cross-project references must be resolved first.

After `tk init`, start the server and sign in, then run `tk setup` to create the Teleport project and choose its description and agent limit. Administrators can also choose the server execution mode. Setup refuses an existing Teleport project without changing settings.

Capture freeform text with `tk import -project Teleport notes.txt` or `tk import -project Teleport` and paste multiple lines, finishing with Ctrl-D. Piped input and `tk import -` work too. Capture creates one Idea in the required named project and preserves up to 20,000 UTF-8 bytes without truncation. `-title` overrides the derived title; `-request-id` reuses the identity printed on stderr for an uncertain retry; `-json` emits the full Idea on stdout. Classification and goal creation follow separately and no agent is started by capture.

Choose **Graph → 2D Map** for the card-and-curve view, or **3D** for the spatial viewer. **Workflows → Map** uses the same visual treatment for phase routes. **Settings → Visual style → Studio** enables the compact menu/table treatment; choose Top or Left under Navigation. The Paper and Instrument styles remain available.

Deploy to the Linux host with:

```sh
make deploy
```

The target detects the server architecture over SSH, builds a static Linux executable, and uses SCP to install under `/home/exedev/teleport` as `exedev@teleport.engineering`. Cross-building from macOS requires Zig or a running Docker installation; native Linux builds use the local compiler. Remote deployment requires Python 3, PostgreSQL 16+ server binaries and existing SSH authentication/host-key trust. It retains a private socket-only cluster below the deployment directory and an owner-only `database.json`. Override `DEPLOY_HOST`, `DEPLOY_DIR` (beneath `/home/exedev`), `DEPLOY_BUILDER` or `DEPLOY_ADDR` when needed.

First deployment creates a random 256-bit admin password and initializes the database offline using `tk init -admin-password-stdin`. Initial credentials are stored in `/home/exedev/teleport/admin.json` and copied to ignored `.deploy/teleport.engineering.json`; both files have mode `0600`. Later deployments retain the database and credentials. A pre-existing database without deployment credentials causes a safe failure rather than a password reset.

To deliberately replace the hosted installation with a fresh one, run `make deploy-reset CONFIRM=reset` when deployment is authorized. The guard is checked before building or contacting the host. The previous installation is retained in a private sibling `teleport-backup-*` directory, and fresh credentials are written to the same protected locations. The reset refuses external PostgreSQL databases and nonempty agent runtime directories; reconcile those separately before retrying. Both public hostnames share the reset. A failed reset can leave the previous installation stopped in its backup directory; inspect the printed path before recovery. `CONFIRM=reset python3 -B tools/deploy.py --reset --dry-run` previews the operation without a network connection.

Existing file-backed deployments require an explicitly supplied archived migration bridge: `python3 -B tools/deploy.py --migration-bridge PATH`. The bridge is hash-verified, creates a consistent private backup under exclusive source ownership, and transfers through `tk init -restore-state-stdin -state-sha256 HASH`. Exact read-back must match before cutover. Previous data and binaries remain available for rollback; a failed release restarts the previous service. Ordinary `tk init` never resets storage, and `-f` is unsupported. Offline `tk init -export-state-stdout` produces private credential-bearing state and must be redirected to an owner-only backup file.

The deployment listener defaults to `0.0.0.0:8000`, matching the exe.dev HTTPS proxy. Both `teleport.engineering` and `teleport.exe.xyz` are trusted HTTPS hosts; `DEPLOY_PUBLIC_HOSTS` adds explicit aliases. Local `tk serve` retains its loopback default; manual proxy installations can pass `--public-host teleport.engineering,teleport.exe.xyz`. The host's proxy must forward to port 8000; port 9999 is reserved by the host's Shelley service. Logs and the owned server PID are stored beneath the deployment directory. `python3 -B tools/deploy.py --dry-run` prints the plan without SSH or copying; `python3 -B tools/deploy.py --build-only --arch amd64` verifies a Linux build without contacting the host.
