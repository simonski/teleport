"""Install and supervise one deployment beneath /home/exedev (invoked over SSH)."""
import hashlib
from contextlib import ExitStack
import json
import os
from pathlib import Path
import re
import shutil
import secrets
import signal
import stat
import subprocess
import sys
import tempfile
import time
import urllib.request
from urllib.parse import parse_qs,urlsplit

HOME_BASE=Path("/home/exedev")


def validate_manifest(manifest):
    root=Path(manifest["root"])
    if not root.is_absolute() or not root.is_relative_to(HOME_BASE) or root==HOME_BASE or ".." in root.parts:
        raise ValueError("Deploy root must be beneath /home/exedev")
    if not re.fullmatch(r"[0-9]{1,20}-[a-f0-9]{12}",manifest["release"]) or not re.fullmatch(r"[a-f0-9]{64}",manifest["sha256"]):
        raise ValueError("Invalid release identity")
    if not re.fullmatch(r"[A-Za-z0-9.+_-]{1,150}",manifest["version"]) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]*",manifest["publicHost"]):
        raise ValueError("Invalid version or public hostname")
    hosts=manifest.get("publicHosts",[manifest["publicHost"]])
    if not isinstance(hosts,list) or not 1<=len(hosts)<=10 or manifest["publicHost"] not in hosts or any(not isinstance(host,str) or not re.fullmatch(r"[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?",host) or ".." in host or any(len(label)>63 or label.startswith("-") or label.endswith("-") for label in host.split(".")) for host in hosts):
        raise ValueError("Invalid public HTTPS hostname aliases")
    if not re.fullmatch(r"(?:127\.0\.0\.1|0\.0\.0\.0):[1-9][0-9]{0,4}",manifest["addr"]) or int(manifest["addr"].rsplit(":",1)[1])>65535:
        raise ValueError("Invalid listen address")
    return root


def secure_json(path,value):
    with tempfile.NamedTemporaryFile(mode="w",dir=path.parent,delete=False,encoding="utf-8") as output:
        temporary=Path(output.name)
        try:
            os.fchmod(output.fileno(),0o600)
            json.dump(value,output,indent=2);output.write("\n");output.flush();os.fsync(output.fileno());output.close();temporary.replace(path)
        finally:
            temporary.unlink(missing_ok=True)


def read_credentials(path):
    info=path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.getuid():
        raise RuntimeError("Credential file must be a regular file owned by the deployment account")
    path.chmod(0o600)
    result=json.loads(path.read_text())
    if result.get("username")!="admin" or not isinstance(result.get("password"),str) or len(result["password"])<32:
        raise RuntimeError("Existing deployment credential file is invalid; refusing to reset it")
    return result


def owned_process(pid,root):
    try:
        executable=Path(os.readlink(f"/proc/{pid}/exe")).resolve()
        command=Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\x00")
        return executable.is_relative_to(root/"releases") and executable.name=="tk" and b"serve" in command
    except (OSError,ValueError):
        return False


def stop_owned(pid,root):
    if not isinstance(pid,int) or pid<=1 or not owned_process(pid,root):
        return
    try:
        os.kill(pid,signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline=time.monotonic()+6
    while time.monotonic()<deadline and owned_process(pid,root):
        time.sleep(.05)
    if owned_process(pid,root):
        os.kill(pid,signal.SIGKILL)


def prepare_database(root):
    from postgres import binaries,start,pg,base_url
    config=root/"database.json"
    if config.exists() or config.is_symlink():
        info=config.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_mode&0o077 or info.st_uid!=os.getuid():
            raise RuntimeError("Database configuration must be an owner-only regular file")
        value=json.loads(config.read_text()).get("url","")
        if not value.startswith(("postgresql://","postgres://")):
            raise RuntimeError("A PostgreSQL database configuration is required")
        if (root/"postgres/data/PG_VERSION").exists():
            start(binaries(),root/"postgres",root/"pgsocket")
        return config,False
    binary=binaries()
    base=start(binary,root/"postgres",root/"pgsocket")
    name="teleport_"+secrets.token_hex(8)
    pg(binary,"createdb","--maintenance-db",base,name)
    secure_json(config,{"url":base_url(root/"pgsocket",name)})
    return config,True


def process_arguments(pid,root):
    if not owned_process(pid,root):return None
    return [part.decode() for part in Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\x00") if part]


def reset_installation(manifest):
    """Archive the owned installation; never delete or touch an external database."""
    if manifest.get("reset") != "reset":
        raise RuntimeError("Reset confirmation is missing")
    root=validate_manifest(manifest)
    if root.is_symlink() or root.resolve()!=root:
        raise RuntimeError("Reset root must not traverse symlinks")
    if root.stat().st_uid!=os.getuid():
        raise RuntimeError("Reset root must be owned by the deployment account")
    release=root/"releases"/manifest["release"]
    binary=release/"tk"
    if release.is_symlink() or binary.is_symlink() or not binary.is_file() or hashlib.sha256(binary.read_bytes()).hexdigest()!=manifest["sha256"]:
        raise RuntimeError("Uploaded reset binary is invalid")
    if subprocess.check_output([str(binary),"version"],text=True,timeout=5).strip()!=manifest["version"]:
        raise RuntimeError("Uploaded reset binary identity differs")
    config=root/"database.json"
    data=root/"postgres/data"
    socket=root/"pgsocket"
    for path in (data,socket):
        if path.is_symlink() or path.exists() and (path.resolve()!=path or not path.is_dir() or path.stat().st_uid!=os.getuid()):
            raise RuntimeError("Reset refuses external or unverified PostgreSQL directories")
    if config.exists() or config.is_symlink():
        info=config.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.getuid() or info.st_mode&0o077:
            raise RuntimeError("Database configuration is not a private owned file")
        url=urlsplit(json.loads(config.read_text()).get("url",""))
        if url.scheme not in ("postgresql","postgres") or url.hostname or parse_qs(url.query).get("host") != [str(socket)] or not (data/"PG_VERSION").is_file():
            raise RuntimeError("Reset refuses external or unverified PostgreSQL databases")
        for path in (data,socket):
            if path.is_symlink() or path.resolve()!=path or not path.is_dir() or path.stat().st_uid!=os.getuid():
                raise RuntimeError("Reset refuses external or unverified PostgreSQL directories")
    runtime=root/"runtime"
    if runtime.is_symlink() or runtime.exists() and (not runtime.is_dir() or any(runtime.iterdir())):
        raise RuntimeError("Reset requires an empty runtime directory; reconcile agent sessions first")
    postgres_running=(data/"postmaster.pid").exists()
    if (data/"postmaster.pid").is_symlink():
        raise RuntimeError("PostgreSQL identity must be a regular file")
    if postgres_running:
        try:
            postgres_pid=int((data/"postmaster.pid").read_text().splitlines()[0])
            executable=Path(os.readlink(f"/proc/{postgres_pid}/exe"))
            arguments=Path(f"/proc/{postgres_pid}/cmdline").read_bytes().split(b"\x00")
            if postgres_pid<=1 or executable.name!="postgres" or str(data).encode() not in arguments or Path(f"/proc/{postgres_pid}").stat().st_uid!=os.getuid():
                raise ValueError("unowned process")
        except (OSError,ValueError,IndexError):
            raise RuntimeError("PostgreSQL process ownership is uncertain; reset aborted") from None
    pid_file=root/"server.json"
    if pid_file.is_symlink() or pid_file.exists() and not pid_file.is_file():
        raise RuntimeError("Server identity must be a regular file")
    recorded=None
    if pid_file.exists():
        recorded=json.loads(pid_file.read_text()).get("pid")
    proc=Path("/proc")
    if sys.platform.startswith("linux") and not proc.is_dir():
        raise RuntimeError("Process ownership cannot be verified without /proc")
    if proc.is_dir():
        for entry in proc.iterdir():
            if entry.name.isdigit() and int(entry.name)!=recorded and owned_process(int(entry.name),root):
                raise RuntimeError("An unrecorded deployment server is running; reset aborted")
    if pid_file.exists():
        pid=recorded
        if not isinstance(pid,int) or pid<=1:
            raise RuntimeError("Server identity is invalid")
        if Path(f"/proc/{pid}").exists() and not owned_process(pid,root):
            raise RuntimeError("Recorded server process is not owned by this deployment")
        stop_owned(pid,root)
        if owned_process(pid,root):
            raise RuntimeError("Owned server did not stop; reset aborted")
    if postgres_running:
        from postgres import binaries,pg
        pg(binaries(),"pg_ctl","-D",data,"-m","fast","-w","stop")
        if (data/"postmaster.pid").exists():
            raise RuntimeError("PostgreSQL did not stop; reset aborted")
    backup=root.parent/(root.name+"-backup-"+secrets.token_hex(8))
    if backup.exists() or backup.is_symlink():
        raise RuntimeError("Backup destination already exists")
    root.chmod(0o700)
    root.rename(backup)
    try:
        root.mkdir(mode=0o700)
        target=root/"releases"/manifest["release"]
        target.parent.mkdir(mode=0o700)
        shutil.move(str(backup/"releases"/manifest["release"]),str(target))
    except BaseException:
        print("Reset interrupted; previous installation retained at "+str(backup),file=sys.stderr,flush=True)
        raise
    print("Previous installation retained at "+str(backup),flush=True)
    return backup


def deploy(manifest):
    root=validate_manifest(manifest)
    root.mkdir(mode=0o700,parents=True,exist_ok=True)
    if root.is_symlink() or root.resolve()!=root:
        raise RuntimeError("Deployment root must not traverse symlinks")
    root.chmod(0o700)
    release=root/"releases"/manifest["release"]
    if release.parent!=root/"releases" or release.resolve()!=release:
        raise ValueError("Invalid release directory")
    binary=release/"tk"
    if not binary.is_file() or binary.is_symlink() or hashlib.sha256(binary.read_bytes()).hexdigest()!=manifest["sha256"]:
        raise RuntimeError("Uploaded binary hash does not match the build")
    binary.chmod(0o700)
    version=subprocess.check_output([str(binary),"version"],text=True,timeout=5).strip()
    if version!=manifest["version"]:
        raise RuntimeError("Uploaded binary identity differs from the build")
    # Validate deployment-owned paths before stopping an existing healthy server.
    pid_file=root/"server.json";current=root/"current";log=root/"server.log";staged=root/".current-next"
    if pid_file.is_symlink() or pid_file.exists() and not pid_file.is_file():
        raise RuntimeError("Server identity must be a regular deployment file")
    if current.exists() and not current.is_symlink():
        raise RuntimeError("Current release must be a managed symlink")
    if log.is_symlink() or log.exists() and not log.is_file():
        raise RuntimeError("Server log must be a regular deployment file")
    if staged.exists() and staged.is_dir() and not staged.is_symlink():
        raise RuntimeError("Release staging path must not be a directory")
    credentials_path=root/"admin.json"
    legacy=root/"database.db"
    if legacy.is_symlink():
        raise RuntimeError("Refusing a symlinked deployment database")
    bridge=release/"migration-tk"
    had_legacy=legacy.exists()
    if legacy.exists():
        digest=manifest.get("migrationBridgeSHA256","")
        if not re.fullmatch(r"[a-f0-9]{64}",digest) or bridge.is_symlink() or not bridge.is_file() or hashlib.sha256(bridge.read_bytes()).hexdigest()!=digest:
            raise RuntimeError("Existing storage requires a verified historical migration bridge; refusing empty initialization")
        bridge.chmod(0o700)
    if credentials_path.exists() or credentials_path.is_symlink():
        credentials=read_credentials(credentials_path)
    elif legacy.exists() or (root/"database.json").exists():
        raise RuntimeError("Database exists without deployment credentials; refusing to reset the admin account")
    else:
        credentials={"server":"https://"+manifest["publicHost"],"username":"admin","password":secrets.token_urlsafe(32),"bootstrapComplete":False}
        secure_json(credentials_path,credentials)
    previous=json.loads(pid_file.read_text()) if pid_file.exists() else None
    previous_args=None
    if previous:previous_args=process_arguments(previous.get("pid"),root)
    previous_release=current.resolve() if current.is_symlink() else None
    config,created_config=prepare_database(root)
    archive=root/"migration"/manifest["release"]
    process=None
    committed=False
    maintenance=ExitStack()
    try:
        if previous:stop_owned(previous.get("pid"),root)
        if legacy.exists():
            from migrate_storage import migration
            maintenance.enter_context(migration(bridge,binary,legacy,config,archive,on_abort=lambda:stop_owned(process.pid,root) if process else None))
        if not credentials.get("bootstrapComplete"):
            subprocess.run([str(binary),"init","-db-config",str(config),"-admin-password-stdin"],input=credentials["password"]+"\n",text=True,check=True,timeout=15)
            credentials["bootstrapComplete"]=True;secure_json(credentials_path,credentials)
        staged.unlink(missing_ok=True);staged.symlink_to(release);staged.replace(current)
        with log.open("ab") as output:
            os.chmod(log,0o600)
            process=subprocess.Popen([str(binary),"serve","-db-config",str(config),"-addr",manifest["addr"],"-public-host",",".join(manifest.get("publicHosts",[manifest["publicHost"]])),"-runtime-dir",str(root/"runtime")],cwd=root,stdin=subprocess.DEVNULL,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)
        secure_json(pid_file,{"pid":process.pid,"version":version,"release":str(release)})
        port=int(manifest["addr"].rsplit(":",1)[1]);deadline=time.monotonic()+10
        healthy=False
        while time.monotonic()<deadline:
            if process.poll() is not None:
                raise RuntimeError("Remote server exited; inspect the protected server.log")
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz",timeout=.5) as response:
                    if json.load(response).get("version")==version:
                        healthy=True;break
            except (OSError,ValueError):pass
            time.sleep(.1)
        if not healthy:raise RuntimeError("Remote health check failed; inspect the protected server.log")
        maintenance.close()
        committed=True
        print(f"PASS: {version} running with PostgreSQL at {manifest['addr']}; credentials: {credentials_path} (0600)",flush=True)
    except BaseException:
        if committed:raise
        if process:stop_owned(process.pid,root)
        failure=sys.exc_info()
        try:maintenance.__exit__(*failure)
        except BaseException as cleanup_error:
            print("Migration cleanup required rollback recovery ("+type(cleanup_error).__name__+")",file=sys.stderr)
        if previous_release:
            staged.unlink(missing_ok=True);staged.symlink_to(previous_release);staged.replace(current)
        if created_config and had_legacy and legacy.exists():
            archive.mkdir(mode=0o700,parents=True,exist_ok=True)
            config.replace(archive/"failed-database.json")
        if previous_args and (not had_legacy or legacy.exists()):
            with log.open("ab") as output:
                restored=subprocess.Popen(previous_args,cwd=root,stdin=subprocess.DEVNULL,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)
            previous["pid"]=restored.pid;secure_json(pid_file,previous)
        raise
    finally:
        maintenance.close()


def main():
    manifest=json.load(sys.stdin)
    root=validate_manifest(manifest)
    root.mkdir(mode=0o700,parents=True,exist_ok=True)
    if root.resolve()!=root:
        raise RuntimeError("Deployment root must not traverse symlinks")
    from tool_history import locked_history
    root=Path(manifest["root"])
    with locked_history(root.parent/("."+root.name+"-deploy")):
        if manifest.get("reset") is not None:
            reset_installation(manifest)
        deploy(manifest)


if __name__=="__main__":
    from tool_history import run
    run(main,tool="tools/deploy_remote.py")
