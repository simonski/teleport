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

Run `tk skill > SKILL.md` to export the bundled agent skill for a harness. No running server or sign-in is required for this export.

Choose **Graph → 2D Map** for the card-and-curve view, or **3D** for the spatial viewer. **Workflows → Map** uses the same visual treatment for phase routes. **Settings → Visual style → Studio** enables the compact menu/table treatment; choose Top or Left under Navigation. The Paper and Instrument styles remain available.

Deploy to the Linux host with:

```sh
make deploy
```

The target detects the server architecture over SSH, builds a static Linux executable, and uses SCP to install under `/home/exedev/teleport` as `exedev@teleport.engineering`. Cross-building from macOS requires Zig or a running Docker installation; native Linux builds use the local compiler. Remote deployment requires Python 3 and existing SSH authentication/host-key trust. Override `DEPLOY_HOST`, `DEPLOY_DIR` (beneath `/home/exedev`), `DEPLOY_BUILDER` or `DEPLOY_ADDR` when needed.

First deployment creates a random 256-bit admin password and initializes the database offline using `tk init --admin-password-stdin`. Initial credentials are stored in `/home/exedev/teleport/admin.json` and copied to ignored `.deploy/teleport.engineering.json`; both files have mode `0600`. Later deployments retain the database and credentials. A pre-existing database without deployment credentials causes a safe failure rather than a password reset.

The listener defaults to `127.0.0.1:9999`; the host's HTTPS reverse proxy should forward `teleport.engineering` to that listener. Logs and the owned server PID are stored beneath the deployment directory. `python3 -B tools/deploy.py --dry-run` prints the plan without SSH or copying; `python3 -B tools/deploy.py --build-only --arch amd64` verifies a Linux build without contacting the host.
