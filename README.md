# teleport

A local workspace for human tickets and supervised agent workflows. Go 1.27.3+, Python 3 and Make are required to build.

```sh
make build
./bin/tk init
./bin/tk serve
```

Open http://localhost:9999 and sign in with the initial local account `admin` / `password`. Initialization starts no listener or agent.

The browser bundles Three.js 0.187.0 under its [MIT license](internal/server/web/vendor/LICENSE). `python3 -B tools/vendor_web.py --verify` checks the pinned artifact hashes offline. No CDN or Node runtime is required by the built application.

Run `make sbom` to generate `sbom.cdx.json`.

Use `make check` for everyday validation. `make race-check` (also `make check-race`) explicitly runs the full race suite at low scheduling priority with two Go execution threads and one package at a time. `make check-full` runs both in order for CI/release validation. PostgreSQL checks use normal tests unless `--race` is supplied.

Run `tk skill > SKILL.md` to export the bundled agent skill for a harness. No running server or sign-in is required for this export.

Choose **Graph → 2D Map** for the card-and-curve view, or **3D** for the spatial viewer. **Workflows → Map** uses the same visual treatment for phase routes. **Settings → Visual style → Studio** enables the compact menu/table treatment; choose Top or Left under Navigation. The Paper and Instrument styles remain available.

Deploy to the Linux host with:

```sh
make deploy
```

The target detects the server architecture over SSH, builds a static Linux executable, and uses SCP to install under `/home/exedev/teleport` as `exedev@teleport.engineering`. Cross-building from macOS requires Zig or a running Docker installation; native Linux builds use the local compiler. Remote deployment requires Python 3 and existing SSH authentication/host-key trust. Override `DEPLOY_HOST`, `DEPLOY_DIR` (beneath `/home/exedev`), `DEPLOY_BUILDER` or `DEPLOY_ADDR` when needed.

First deployment creates a random 256-bit admin password and initializes the database offline using `tk init --admin-password-stdin`. Initial credentials are stored in `/home/exedev/teleport/admin.json` and copied to ignored `.deploy/teleport.engineering.json`; both files have mode `0600`. Later deployments retain the database and credentials. A pre-existing database without deployment credentials causes a safe failure rather than a password reset.

The deployment listener defaults to `0.0.0.0:8000`, matching the exe.dev HTTPS proxy. Both `teleport.engineering` and `teleport.exe.xyz` are trusted HTTPS hosts; `DEPLOY_PUBLIC_HOSTS` adds explicit aliases. Local `tk serve` retains its loopback default; manual proxy installations can pass `--public-host teleport.engineering,teleport.exe.xyz`. The host's proxy must forward to port 8000; port 9999 is reserved by the host's Shelley service. Logs and the owned server PID are stored beneath the deployment directory. `python3 -B tools/deploy.py --dry-run` prints the plan without SSH or copying; `python3 -B tools/deploy.py --build-only --arch amd64` verifies a Linux build without contacting the host.
