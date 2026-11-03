"""Own disposable PostgreSQL clusters and the explicitly started local development database."""
import argparse
from contextlib import contextmanager
from contextvars import ContextVar
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.parse import parse_qsl, unquote, urlencode, urlsplit, urlunsplit

_deadline = ContextVar("postgres_deadline", default=None)


def binaries():
    candidates = [Path(os.environ["TELEPORT_POSTGRES_BIN"])] if os.environ.get("TELEPORT_POSTGRES_BIN") else []
    if shutil.which("initdb"):
        candidates.append(Path(shutil.which("initdb")).parent)
    candidates += [Path("/Applications/Postgres.app/Contents/Versions/latest/bin"), Path("/opt/homebrew/opt/postgresql@17/bin"), Path("/usr/local/opt/postgresql@17/bin")]
    candidates += sorted(Path("/usr/lib/postgresql").glob("*/bin"), reverse=True)
    result = next((path for path in candidates if (path / "initdb").is_file()), None)
    if result is None:
        raise RuntimeError("Install PostgreSQL server binaries or set TELEPORT_POSTGRES_BIN")
    return result


def pg(binary, name, *arguments):
    environment = {key: value for key, value in os.environ.items() if not key.startswith("PG")}
    safe=[]
    for value in map(str,arguments):
        if value.startswith(("postgresql://","postgres://")):
            parts=urlsplit(value)
            if parts.password is not None:
                environment["PGPASSWORD"]=unquote(parts.password)
                user=parts.netloc.rsplit("@",1)[0].split(":",1)[0]
                parts=parts._replace(netloc=user+"@"+parts.netloc.rsplit("@",1)[1])
            query=[]
            for key,item in parse_qsl(parts.query,keep_blank_values=True):
                if key=="password":environment["PGPASSWORD"]=item
                else:query.append((key,item))
            value=urlunsplit(parts._replace(query=urlencode(query)))
        safe.append(value)
    deadline=_deadline.get()
    timeout=30 if deadline is None else max(.001,min(30,deadline-time.monotonic()))
    result = subprocess.run([str(binary / name), *safe], env=environment, capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError("PostgreSQL " + name + " failed: " + result.stderr.strip())
    return result.stdout


def base_url(socket, database="postgres"):
    return "postgresql://teleport@/" + database + "?" + urlencode({"host": str(socket), "sslmode": "disable"})


def start(binary, folder, socket):
    folder.mkdir(parents=True, exist_ok=True, mode=0o700)
    socket.mkdir(parents=True, exist_ok=True, mode=0o700)
    for path in (folder, socket):
        if path.is_symlink() or path.stat().st_uid != os.getuid():
            raise RuntimeError("PostgreSQL directories must be owned regular directories")
        path.chmod(0o700)
    data = folder / "data"
    if not (data / "PG_VERSION").is_file():
        pg(binary, "initdb", "-D", data, "--auth=trust", "--username=teleport", "--encoding=UTF8", "--no-locale")
    if not (data / "postmaster.pid").exists():
        options = "-c listen_addresses='' -c unix_socket_permissions=0700 -c unix_socket_directories=" + str(socket)
        pg(binary, "pg_ctl", "-D", data, "-l", folder / "server.log", "-o", options, "-w", "start")
    return base_url(socket)


@contextmanager
def cluster(environment=None, deadline=None, work_deadline=None):
    environment = dict(os.environ if environment is None else environment)
    if environment.get("TELEPORT_TEST_POSTGRES"):
        yield environment
        return
    binary = binaries()
    with tempfile.TemporaryDirectory(prefix="tkpg-", dir="/tmp") as temporary:
        folder = Path(temporary)
        started = False
        token=_deadline.set(work_deadline or deadline)
        try:
            url = start(binary, folder, folder)
            started = True
            yield dict(environment, TELEPORT_TEST_POSTGRES=url)
        finally:
            try:
                _deadline.set(deadline)
                if started or (folder/"data/postmaster.pid").exists():
                    pg(binary, "pg_ctl", "-D", folder / "data", "-m", "immediate", "-w", "stop")
            finally:_deadline.reset(token)


@contextmanager
def database(explicit=None, environment=None):
    if explicit:
        yield explicit
        return
    environment = os.environ if environment is None else environment
    base = environment.get("TELEPORT_TEST_POSTGRES")
    if not base:
        raise RuntimeError("A disposable PostgreSQL cluster is required; use the supervised tool entry point")
    binary = binaries()
    name = "tk_tool_" + secrets.token_hex(10)
    pg(binary, "createdb", "--maintenance-db", base, name)
    parts = urlsplit(base)
    url = urlunsplit(parts._replace(path="/" + name))
    try:
        yield url
    finally:
        pg(binary, "dropdb", "--force", "--maintenance-db", base, name)


def run_with_database(action):
    if "--database" in sys.argv or any(value.startswith("--database=") for value in sys.argv):
        return action(None)
    with cluster() as environment, database(environment=environment) as url:
        return action(url)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("up", "down"))
    args = parser.parse_args()
    root = Path.home() / ".teleport"
    folder = root / "postgres"
    socket = Path("/tmp") / ("teleport-pg-" + str(os.getuid()))
    binary = binaries()
    if args.action == "down":
        if (folder / "data/postmaster.pid").exists():
            pg(binary, "pg_ctl", "-D", folder / "data", "-m", "fast", "-w", "stop")
        print("Local development PostgreSQL stopped")
        return
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    root.chmod(0o700)
    config = root / "database.json"
    if (root / "database.db").exists() and not config.exists():
        raise RuntimeError("Existing installation requires explicit data migration before local setup")
    if config.exists():
        value = json.loads(config.read_text()).get("url")
        parts=urlsplit(value or "")
        if dict(parse_qsl(parts.query)).get("host")!=str(socket) or not (folder/"data/PG_VERSION").is_file():
            raise RuntimeError("Database configuration already exists; refusing to replace it")
        start(binary, folder, socket)
        print("Local PostgreSQL ready; existing configuration retained: " + str(config))
        return
    base = start(binary, folder, socket)
    pg(binary, "createdb", "--maintenance-db", base, "teleport")
    value = base_url(socket, "teleport")
    with open(config,"x", opener=lambda path, flags: os.open(path, flags, 0o600)) as output:
        json.dump({"url": value}, output)
    print("Local PostgreSQL ready; configuration: " + str(config))


if __name__ == "__main__":
    from tool_history import run
    run(main)
