"""Install and supervise one deployment beneath /home/exedev (invoked over SSH)."""
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import signal
import stat
import subprocess
import sys
import tempfile
import time
import urllib.request

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
    database=root/"database.db"
    if database.is_symlink():
        raise RuntimeError("Refusing a symlinked deployment database")
    if credentials_path.exists() or credentials_path.is_symlink():
        credentials=read_credentials(credentials_path)
    elif database.exists():
        raise RuntimeError("Database exists without deployment credentials; refusing to reset the admin account")
    else:
        credentials={"server":"https://"+manifest["publicHost"],"username":"admin","password":secrets.token_urlsafe(32),"bootstrapComplete":False}
        secure_json(credentials_path,credentials)
    # Save the secret before initialization so an interrupted first deploy can
    # resume using the same password. Initialization is offline and never -f.
    if not credentials.get("bootstrapComplete"):
        subprocess.run([str(binary),"init","--db",str(database),"--admin-password-stdin"],input=credentials["password"]+"\n",text=True,check=True,timeout=15)
        credentials["bootstrapComplete"]=True;secure_json(credentials_path,credentials)
    if pid_file.exists():
        stop_owned(json.loads(pid_file.read_text()).get("pid"),root)
    staged.unlink(missing_ok=True);staged.symlink_to(release);staged.replace(current)
    with log.open("ab") as output:
        os.chmod(log,0o600)
        process=subprocess.Popen([str(binary),"serve","--db",str(database),"--addr",manifest["addr"],"--public-host",",".join(manifest.get("publicHosts",[manifest["publicHost"]])),"--runtime-dir",str(root/"runtime")],cwd=root,stdin=subprocess.DEVNULL,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)
    secure_json(pid_file,{"pid":process.pid,"version":version,"release":str(release)})
    port=int(manifest["addr"].rsplit(":",1)[1]);deadline=time.monotonic()+10
    while time.monotonic()<deadline:
        if process.poll() is not None:
            raise RuntimeError("Remote server exited; inspect the protected server.log")
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz",timeout=.5) as response:
                if json.load(response).get("version")==version:
                    print(f"PASS: {version} running at {manifest['addr']}; credentials: {credentials_path} (0600)",flush=True)
                    return
        except (OSError,ValueError):
            pass
        time.sleep(.1)
    stop_owned(process.pid,root)
    raise RuntimeError("Remote health check failed; inspect the protected server.log")


def main():
    manifest=json.load(sys.stdin)
    root=validate_manifest(manifest)
    root.mkdir(mode=0o700,parents=True,exist_ok=True)
    if root.resolve()!=root:
        raise RuntimeError("Deployment root must not traverse symlinks")
    from tool_history import locked_history
    with locked_history(Path(manifest["root"])/"deploy"):
        deploy(manifest)


if __name__=="__main__":
    from tool_history import run
    run(main,tool="tools/deploy_remote.py")
