"""Migrate the local installation with exact verification, owned-process restart and rollback."""
import argparse
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import secrets
import subprocess
import tempfile
import time
import urllib.request

from deploy_remote import secure_json
from migrate_storage import migration
from postgres import binaries,start,pg,base_url

ROOT=Path(__file__).resolve().parents[1]


def owned_command(source,installed):
    result=subprocess.run(["lsof","-t",str(source)],capture_output=True,text=True)
    owners=set(result.stdout.split())
    if not owners:return None,None
    if len(owners)!=1:raise RuntimeError("Migration requires one known installation owner")
    pid=int(owners.pop())
    files=subprocess.check_output(["lsof","-a","-p",str(pid),"-d","txt","-Fn"],text=True)
    if "n"+str(installed.resolve()) not in files.splitlines():
        raise RuntimeError("Current database owner is not the selected installed tk binary")
    command=shlex.split(subprocess.check_output(["ps","-p",str(pid),"-o","command="],text=True).strip())
    if len(command)<2 or command[1]!="serve":raise RuntimeError("Database owner is not tk serve")
    if len(command)!=2:
        raise RuntimeError("Stop configured local servers explicitly before migration; process-list text cannot safely reconstruct quoted arguments")
    command[0]=str(installed)
    return pid,command


def stop(pid,expected=None):
    if pid is None:return
    if expected is not None:
        current=subprocess.run(["lsof","-a","-p",str(pid),"-d","txt","-Fn"],capture_output=True,text=True)
        if current.returncode:return
        if "n"+str(expected.resolve()) not in current.stdout.splitlines():
            raise RuntimeError("Process identity changed; refusing to signal an unrelated owner")
    os.kill(pid,signal.SIGTERM)
    deadline=time.monotonic()+10
    while time.monotonic()<deadline:
        result=subprocess.run(["ps","-p",str(pid),"-o","stat="],capture_output=True,text=True)
        if result.returncode or result.stdout.strip().startswith("Z"):return
        time.sleep(.05)
    raise RuntimeError("Owned local server did not stop; migration was not attempted")


def install(source,destination):
    with tempfile.NamedTemporaryFile(dir=destination.parent,delete=False) as temporary:
        staged=Path(temporary.name)
    try:
        shutil.copyfile(source,staged);staged.chmod(0o755);staged.replace(destination)
    finally:staged.unlink(missing_ok=True)


def launch(command,log):
    with log.open("ab") as output:
        os.chmod(log,0o600)
        return subprocess.Popen(command,stdin=subprocess.DEVNULL,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bridge",type=Path,required=True)
    parser.add_argument("--installed",type=Path,default=Path(shutil.which("tk") or str(Path.home()/"go/bin/tk")))
    args=parser.parse_args()
    root=(Path.home()/".teleport").resolve()
    source=root/"database.db"
    config=root/"database.json"
    if not source.is_file() or source.is_symlink():raise RuntimeError("A regular legacy local installation is required")
    if config.exists() or config.is_symlink():raise RuntimeError("Existing PostgreSQL configuration requires explicit operator reconciliation")
    installed=args.installed.absolute()
    if installed.is_symlink() or not installed.is_file():raise RuntimeError("Installed tk must be a regular file")
    pid,previous=owned_command(source,installed)
    archive=root/"migration"/(time.strftime("%Y%m%dT%H%M%SZ",time.gmtime())+"-local")
    archive.mkdir(mode=0o700,parents=True,exist_ok=False)
    shutil.copy2(installed,archive/"previous-tk")
    pg_bin=binaries()
    socket=Path("/tmp")/("teleport-pg-"+str(os.getuid()))
    base=start(pg_bin,root/"postgres",socket)
    name="teleport_"+secrets.token_hex(8)
    pg(pg_bin,"createdb","--maintenance-db",base,name)
    secure_json(config,{"url":base_url(socket,name)})
    process=None
    changed=False
    stopped=False
    committed=False
    try:
        stop(pid,installed);stopped=True
        with migration(args.bridge.resolve(),ROOT/"bin/tk",source,config,archive,on_abort=lambda:stop(process.pid) if process and process.poll() is None else None) as verified:
            install(ROOT/"bin/tk",installed);changed=True
            if previous:
                command=[]
                skip=False
                for argument in previous:
                    if skip:skip=False;continue
                    if argument in ("-db","--db","-db-config","--db-config"):skip=True;continue
                    if argument.startswith(("-db=","--db=","-db-config=","--db-config=")):continue
                    command.append(argument)
                command += ["-db-config",str(config)]
                address="localhost:9999"
                for index,argument in enumerate(command):
                    if argument in ("-addr","--addr"):address=command[index+1]
                    elif argument.startswith(("-addr=","--addr=")):address=argument.split("=",1)[1]
                if address.startswith("0.0.0.0:"):address=address.replace("0.0.0.0:","127.0.0.1:",1)
                expected=subprocess.check_output([str(installed),"version"],text=True).strip()
                process=launch(command,root/"migration-server.log")
                try:
                    deadline=time.monotonic()+10
                    while True:
                        if process.poll() is not None:raise RuntimeError("New local service exited; rollback required")
                        try:
                            with urllib.request.urlopen("http://"+address+"/healthz",timeout=.5) as response:
                                if response.status==200 and json.load(response).get("version")==expected:break
                        except (OSError,ValueError):pass
                        if time.monotonic()>deadline:raise RuntimeError("New local service health did not confirm")
                        time.sleep(.05)
                except BaseException:
                    if process.poll() is None:stop(process.pid)
                    raise
            try:secure_json(root/"local-migration.json",dict(verified,archive=str(archive),pid=process.pid if process else None))
            except BaseException:
                if process and process.poll() is None:stop(process.pid)
                raise
        committed=True
        print("PASS: local installation migrated to PostgreSQL; installed binary updated; rollback archive: "+str(archive))
    except BaseException:
        if committed:raise
        if process and process.poll() is None:stop(process.pid)
        if source.exists():
            if changed:install(archive/"previous-tk",installed)
            if config.exists():config.replace(archive/"failed-database.json")
            if stopped and previous:launch(previous,root/"migration-server.log")
        raise


if __name__=="__main__":
    from tool_history import run
    run(main)
