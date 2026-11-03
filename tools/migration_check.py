"""Rehearse exact legacy-state cutover, ownership fencing and rollback with synthetic data."""
import argparse
import hashlib
import json
import os
import signal
from pathlib import Path
import subprocess
import tempfile

from deploy_remote import secure_json
from migrate_storage import migration
from postgres import cluster, database

ROOT=Path(__file__).resolve().parents[1]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bridge",type=Path,required=True)
    args=parser.parse_args()
    bridge=args.bridge.resolve()
    binary=ROOT/"bin/tk"
    with tempfile.TemporaryDirectory(prefix="tk-migrate-",dir="/tmp") as temporary,cluster() as environment,database(environment=environment) as url:
        folder=Path(temporary).resolve()
        source=folder/"installation.legacy"
        config=folder/"database.json"
        secure_json(config,{"url":url})
        legacy_environment=dict(os.environ,TELEPORT_DB=str(source))
        result=subprocess.run([str(bridge),"init","-demo"],env=legacy_environment,capture_output=True,timeout=20)
        if result.returncode:raise RuntimeError("Synthetic source initialization failed")
        class RollbackFixture(Exception):pass
        try:
            with migration(bridge,binary,source,config,folder/"rollback"):
                assert not source.exists(),"old source was not retired before new service admission"
                result=subprocess.run([str(bridge),"init","-export-state-stdout"],env=legacy_environment,capture_output=True,timeout=5)
                assert result.returncode,"a second source owner was admitted during cutover"
                raise RollbackFixture()
        except RollbackFixture:pass
        assert source.is_file(),"rollback did not restore the previous authority"
        abort_observation=[]
        try:
            with migration(bridge,binary,source,config,folder/"exited-bridge",on_abort=lambda:abort_observation.append(source.exists())) as verified:
                os.kill(verified["ownerPid"],signal.SIGKILL)
        except (BrokenPipeError,RuntimeError):pass
        else:raise AssertionError("unexpected bridge exit was accepted")
        assert abort_observation==[False],"new-service abort did not precede source restoration"
        assert source.is_file(),"bridge failure did not restore the previous authority"
        with migration(bridge,binary,source,config,folder/"success") as verified:
            assert not source.exists()
        environment=dict(os.environ)
        environment.pop("TELEPORT_DB",None)
        result=subprocess.run([str(binary),"init","-db-config",str(config),"-export-state-stdout"],env=environment,capture_output=True,timeout=10,check=True)
        assert hashlib.sha256(result.stdout).hexdigest()==verified["sha256"]
        state=json.loads(result.stdout)
        assert state["installationId"]==verified["installationId"] and len(state["tickets"])>5
        assert not source.exists()
        for path in (folder/"success").glob("*.json"):
            assert path.stat().st_mode&0o077==0
        print("PASS: synthetic legacy cutover, exact complete state, ownership fencing, source retirement, retry and rollback; zero model calls")


if __name__=="__main__":
    from tool_history import run
    run(main)
