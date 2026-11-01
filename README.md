# teleport

A local workspace for human tickets and supervised agent workflows. Go 1.26.1+, Python 3 and Make are required to build.

```sh
make build
./bin/tk init
./bin/tk serve
```

Open http://localhost:9999 and sign in with the initial local account `admin` / `password`. Initialization starts no listener or agent. See [USER_GUIDE.md](USER_GUIDE.md), [API_CONTRACT.md](API_CONTRACT.md) and [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) for actual support and phase evidence.
