"""Verify public deployment aliases, HTTPS origins and an optional shared sign-in."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.error
import urllib.request

ROOT=Path(__file__).resolve().parents[1]


def request(url,data=None,origin=None):
    headers={"Accept":"application/json"}
    if data is not None:headers["Content-Type"]="application/json"
    if origin:headers["Origin"]=origin
    try:
        with urllib.request.urlopen(urllib.request.Request(url,data=json.dumps(data).encode() if data is not None else None,headers=headers),timeout=8) as response:
            return response.status,response.read().decode()
    except urllib.error.HTTPError as error:
        return error.code,error.read().decode()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host",action="append",dest="hosts",help="Bare HTTPS hostname; repeat for aliases")
    parser.add_argument("--expected-version")
    parser.add_argument("--credentials",type=Path,help="Protected bootstrap credential file; passwords are read privately and passed through stdin")
    args=parser.parse_args();hosts=args.hosts or ["teleport.engineering","teleport.exe.xyz"]
    if any(not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]*",host) for host in hosts):parser.error("Use bare HTTPS hostnames")
    versions=set();installations=set()
    credentials=None
    if args.credentials:
        if args.credentials.is_symlink() or not args.credentials.is_file() or args.credentials.stat().st_mode&0o077:raise RuntimeError("Use an owner-only regular credential file")
        credentials=json.loads(args.credentials.read_text())
    with tempfile.TemporaryDirectory(prefix="teleport-deploy-verify-") as temporary:
        for index,host in enumerate(hosts):
            base="https://"+host
            status,body=request(base+"/healthz")
            if status!=200:raise RuntimeError("Public deployment health check failed for "+host)
            health=json.loads(body);version=health.get("version");versions.add(version)
            if health.get("status")!="ok" or args.expected_version and version!=args.expected_version:raise RuntimeError("Public deployment identity differs from the release")
            status,body=request(base+"/")
            if status!=200 or '/backlog.js' not in body:raise RuntimeError("Current web assets are unavailable on "+host)
            probe={"username":"__deployment_origin_probe__","password":"invalid-deployment-probe"}
            if request(base+"/api/tools/login",probe,base)[0]!=401:raise RuntimeError("Same-origin HTTPS sign-in is not accepted on "+host)
            if len(hosts)>1 and request(base+"/api/tools/login",probe,"https://"+hosts[(index+1)%len(hosts)])[0]!=403:raise RuntimeError("Cross-alias browser origin was not rejected")
            if credentials:
                config=str(Path(temporary)/(str(index)+".json"))
                common=["--server",base,"--config",config]
                result=subprocess.run([str(ROOT/"bin/tk"),"login",*common,"--username",credentials["username"],"--password-stdin"],input=credentials["password"]+"\n",capture_output=True,text=True,timeout=10)
                if result.returncode:raise RuntimeError("Protected credential sign-in could not be verified on "+host)
                try:
                    result=subprocess.run([str(ROOT/"bin/tk"),"getSession",*common,"-json"],capture_output=True,text=True,timeout=10,check=True)
                    installations.add(json.loads(result.stdout)["installationId"])
                finally:
                    subprocess.run([str(ROOT/"bin/tk"),"logout",*common],capture_output=True,text=True,timeout=10,check=True)
            print(f"PASS: {host} HTTPS health {version}, current web assets and browser origin checks",flush=True)
    if len(versions)!=1 or credentials and len(installations)!=1:raise RuntimeError("Aliases did not identify the same deployment")
    if credentials:print("PASS: both aliases sign in to the same installation; verification sessions signed out",flush=True)


if __name__=="__main__":
    from tool_history import run
    run(main)
