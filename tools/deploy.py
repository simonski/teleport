"""Build a Linux tk, copy it under /home/exedev, and start the remote installation."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time

from build import local_version

ROOT=Path(__file__).resolve().parents[1]
ARCHES={"x86_64":"amd64","amd64":"amd64","aarch64":"arm64","arm64":"arm64"}


def validate_target(host,directory):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+@[A-Za-z0-9][A-Za-z0-9.-]*",host) or host.startswith("-"):
        raise ValueError("Use an SSH target such as exedev@teleport.engineering")
    path=PurePosixPath(directory)
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+",directory) or not path.is_absolute() or ".." in path.parts or not path.is_relative_to("/home/exedev") or path==PurePosixPath("/home/exedev"):
        raise ValueError("Deployment must be a directory below /home/exedev")
    return str(path)


def linux_build(arch,builder="auto"):
    if arch not in ("amd64","arm64"):
        raise ValueError("Supported Linux architectures: amd64, arm64")
    output=ROOT/"bin/deploy"/("linux-"+arch)/"tk"
    output.parent.mkdir(parents=True,exist_ok=True)
    version=os.environ.get("TELEPORT_BUILD_VERSION") or local_version(ROOT,0).replace("dev.0","deploy")
    environment=dict(os.environ,GOOS="linux",GOARCH=arch,CGO_ENABLED="1")
    environment.setdefault("CGO_CFLAGS","-Os -g0")
    native=platform.system()=="Linux" and ARCHES.get(platform.machine())==arch
    chosen=("native" if native else "zig" if shutil.which("zig") else "docker") if builder=="auto" else builder
    subprocess.run([sys.executable,"-B",str(ROOT/"tools/pack_web.py")],cwd=ROOT,check=True)
    flags=["-trimpath","-tags","netgo,osusergo","-ldflags",f"-s -w -X main.version={version} -linkmode external -extldflags -static"]
    if chosen=="zig":
        zig=shutil.which("zig")
        if not zig:
            raise RuntimeError("Install Zig or select --builder docker for Linux CGO cross-compilation")
        target=("x86_64" if arch=="amd64" else "aarch64")+"-linux-musl"
        environment.update(CC=shlex.quote(zig)+" cc -target "+target,CXX=shlex.quote(zig)+" c++ -target "+target)
        command=["go","build",*flags,"-o",str(output),"./cmd/tk"]
    elif chosen=="native" and native:
        command=["go","build",*flags,"-o",str(output),"./cmd/tk"]
    elif chosen=="docker":
        if not shutil.which("docker"):
            raise RuntimeError("Linux CGO build requires Zig, Docker, or a native Linux compiler")
        cache=ROOT/".cache/tools/deploy";cache.mkdir(parents=True,exist_ok=True)
        command=["docker","run","--rm","--platform","linux/"+arch,"--user",f"{os.getuid()}:{os.getgid()}",
                 "-v",str(ROOT)+":/src:ro","-v",str(output.parent)+":/out","-v",str(cache)+":/cache","-w","/src",
                 "-e","CGO_ENABLED=1","-e","CGO_CFLAGS","-e","GOCACHE=/cache/build","-e","GOMODCACHE=/cache/mod","-e","GOPATH=/cache/gopath",
                 "golang:1.27.3-bookworm","go","build",*flags,"-o","/out/tk","./cmd/tk"]
    else:
        raise RuntimeError("Native builds require Linux with the target architecture")
    subprocess.run(command,cwd=ROOT,env=environment,check=True,timeout=600)
    header=output.read_bytes()[:20]
    machine=int.from_bytes(header[18:20],"little")
    if header[:4]!=b"\x7fELF" or machine!={"amd64":62,"arm64":183}[arch]:
        raise RuntimeError("Build did not produce the requested Linux ELF executable")
    print(f"Built Linux {arch}: {output.relative_to(ROOT)} ({version})",flush=True)
    return output,version


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host",default=os.environ.get("DEPLOY_HOST","exedev@teleport.engineering"))
    parser.add_argument("--directory",default=os.environ.get("DEPLOY_DIR","/home/exedev/teleport"))
    parser.add_argument("--arch",choices=("amd64","arm64"))
    parser.add_argument("--builder",choices=("auto","native","zig","docker"),default=os.environ.get("DEPLOY_BUILDER","auto"))
    parser.add_argument("--addr",default=os.environ.get("DEPLOY_ADDR","0.0.0.0:8000"))
    parser.add_argument("--public-hosts",default=os.environ.get("DEPLOY_PUBLIC_HOSTS",""),help="Additional comma-separated HTTPS hostnames")
    parser.add_argument("--build-only",action="store_true")
    parser.add_argument("--dry-run",action="store_true")
    parser.add_argument("--migration-bridge",type=Path,help="Archived historical init bridge for an existing file-backed deployment")
    parser.add_argument("--reset",action="store_true",help="Archive the existing installation and initialize a fresh one; requires CONFIRM=reset")
    args=parser.parse_args()
    if args.reset and os.environ.get("CONFIRM") != "reset":
        parser.error("reset requires CONFIRM=reset; no connection or build was started")
    if args.reset and (args.build_only or args.migration_bridge):
        parser.error("reset cannot be combined with build-only or migration")
    directory=validate_target(args.host,args.directory)
    primary=args.host.split("@",1)[1].lower()
    aliases=["teleport.engineering","teleport.exe.xyz"] if primary in ("teleport.engineering","teleport.exe.xyz") else []
    if args.public_hosts:aliases.extend(args.public_hosts.split(","))
    public_hosts=list(dict.fromkeys([primary,*[host.strip().lower() for host in aliases]]))
    if len(public_hosts)>10 or any(not re.fullmatch(r"[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?",host) or ".." in host or any(len(label)>63 or label.startswith("-") or label.endswith("-") for label in host.split(".")) for host in public_hosts):
        parser.error("Use up to ten bare public HTTPS hostnames")
    if not re.fullmatch(r"(?:127\.0\.0\.1|0\.0\.0\.0):[1-9][0-9]{0,4}",args.addr) or int(args.addr.rsplit(":",1)[1])>65535:
        parser.error("Use a Linux listen address such as 127.0.0.1:9999")
    if args.dry_run:
        if args.reset: print("RESET: archive the existing installation, stop owned processes, and create a fresh database and credentials")
        print(f"Build Linux {args.arch or 'architecture detected over SSH'}; scp to {args.host}:{directory}/releases; initialize only if needed; start {args.addr}; HTTPS hosts: {', '.join(public_hosts)}; retrieve protected credentials to .deploy/")
        return
    ssh=["ssh","-o","BatchMode=yes","-o","ConnectTimeout=10",args.host]
    arch=args.arch
    if not args.build_only:
        system=subprocess.check_output([*ssh,"uname -sm"],text=True,timeout=15).split()
        if len(system)!=2 or system[0]!="Linux" or system[1] not in ARCHES:
            raise RuntimeError("Deployment target must be supported Linux amd64 or arm64")
        actual=ARCHES[system[1]]
        if arch and arch!=actual:
            raise RuntimeError("Requested binary architecture differs from the remote Linux architecture")
        arch=actual
    if not arch:
        parser.error("--build-only requires --arch")
    binary,version=linux_build(arch,args.builder)
    if args.build_only:
        return
    release=f"{int(time.time())}-{hashlib.sha256(binary.read_bytes()).hexdigest()[:12]}"
    remote=directory+"/releases/"+release
    subprocess.run([*ssh,shlex.join(["mkdir","-p","--",remote])],check=True,timeout=15)
    uploads=[(binary,"tk"),(ROOT/"tools/deploy_remote.py","deploy_remote.py"),(ROOT/"tools/tool_history.py","tool_history.py"),(ROOT/"tools/postgres.py","postgres.py"),(ROOT/"tools/migrate_storage.py","migrate_storage.py")]
    if args.migration_bridge:uploads.append((args.migration_bridge,"migration-tk"))
    for source,destination in uploads:
        subprocess.run(["scp","-C","-q","-o","BatchMode=yes","-o","ConnectTimeout=10",str(source),args.host+":"+shlex.quote(remote+"/"+destination)],check=True,timeout=120)
    manifest={"root":directory,"release":release,"version":version,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"addr":args.addr,"publicHost":primary,"publicHosts":public_hosts}
    if args.reset:manifest["reset"]="reset"
    if args.migration_bridge:manifest["migrationBridgeSHA256"]=hashlib.sha256(args.migration_bridge.read_bytes()).hexdigest()
    subprocess.run([*ssh,shlex.join(["env","TELEPORT_TOOL_HISTORY="+directory+"/tool_calls_history.csv","python3",remote+"/deploy_remote.py"])],input=json.dumps(manifest),text=True,check=True,timeout=120)
    credentials=ROOT/".deploy";credentials.mkdir(mode=0o700,exist_ok=True)
    if credentials.is_symlink():
        raise RuntimeError("Refusing a symlinked local deployment credential directory")
    credentials.chmod(0o700)
    destination=credentials/(args.host.split("@",1)[1]+".json")
    if destination.is_symlink():
        raise RuntimeError("Refusing a symlinked local deployment credential file")
    with tempfile.NamedTemporaryFile(dir=credentials,delete=False) as staged:
        temporary=Path(staged.name)
    try:
        subprocess.run(["scp","-q","-o","BatchMode=yes",args.host+":"+shlex.quote(directory+"/admin.json"),str(temporary)],check=True,timeout=20)
        temporary.chmod(0o600);temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)
    print(f"PASS: deployed {version} to {args.host}:{directory}; initial admin credentials: {destination.relative_to(ROOT)} (0600)")


if __name__=="__main__":
    from tool_history import run
    run(main)
