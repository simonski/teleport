"""Verify deployment path safety, credential permissions and first-run-only bootstrap."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch,MagicMock

from deploy import validate_target
from deploy_remote import deploy,read_credentials,reset_installation,secure_json,validate_manifest


class DeployTest(unittest.TestCase):
    def test_reset_guard_precedes_build_and_network(self):
        import os, subprocess, sys
        environment=dict(os.environ,CONFIRM="")
        result=subprocess.run([sys.executable,"-B",str(Path(__file__).with_name("deploy.py")),"--reset"],env=environment,capture_output=True,text=True,timeout=3)
        self.assertNotEqual(result.returncode,0)
        self.assertIn("CONFIRM=reset",result.stderr)

    def test_reset_archives_old_data_and_keeps_uploaded_release(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture");(root/"database.db").write_bytes(b"old data")
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering","reset":"reset"}
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.subprocess.check_output",return_value="fixture"):
                backup=reset_installation(manifest)
            self.assertEqual((backup/"database.db").read_bytes(),b"old data")
            self.assertEqual((root/"releases"/release.name/"tk").read_bytes(),b"fixture")
            self.assertFalse((root/"database.db").exists())

    def test_reset_rejects_external_database_and_unreconciled_runtime_before_stopping(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture")
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering","reset":"reset"}
            config=root/"database.json";secure_json(config,{"url":"postgresql://example.test/production"})
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.stop_owned") as stop:
                with self.assertRaisesRegex(RuntimeError,"external"):reset_installation(manifest)
                config.unlink();(root/"runtime").mkdir();(root/"runtime/session").write_text("owned work")
                with self.assertRaisesRegex(RuntimeError,"runtime"):reset_installation(manifest)
                stop.assert_not_called()
                self.assertTrue(binary.exists())

    def test_destination_is_below_the_requested_home_without_shell_fragments(self):
        self.assertEqual(validate_target("exedev@teleport.engineering","/home/exedev/teleport"),"/home/exedev/teleport")
        for host,path in (("-oProxyCommand=bad","/home/exedev/teleport"),("exedev@teleport.engineering","/tmp/app"),("exedev@teleport.engineering","/home/exedev/../other"),("exedev@teleport.engineering","/home/exedev/$(bad)")):
            with self.assertRaises(ValueError):validate_target(host,path)

    def test_secret_files_are_private_and_symlinks_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary);path=root/"admin.json"
            value={"username":"admin","password":"x"*43}
            secure_json(path,value)
            self.assertEqual(path.stat().st_mode&0o777,0o600)
            self.assertEqual(read_credentials(path),value)
            link=root/"link";link.symlink_to(path)
            with self.assertRaises(RuntimeError):read_credentials(link)

    def test_first_bootstrap_uses_stdin_and_redeploy_preserves_credentials_and_database(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture binary")
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:9999","publicHost":"teleport.engineering","publicHosts":["teleport.engineering","teleport.exe.xyz"]}
            process=MagicMock(pid=12345);process.poll.return_value=None
            class Response:
                def __enter__(self):return self
                def __exit__(self,*args):pass
                def read(self):return b'{"version":"fixture"}'
            def prepare(root):
                config=root/"database.json"
                existed=config.exists()
                if not existed:secure_json(config,{"url":"postgresql:///fixture"})
                return config,not existed
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.prepare_database",side_effect=prepare),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.subprocess.run") as run,patch("deploy_remote.subprocess.Popen",return_value=process) as popen,patch("deploy_remote.urllib.request.urlopen",return_value=Response()),patch("deploy_remote.stop_owned"):
                deploy(manifest)
                self.assertIn("teleport.engineering,teleport.exe.xyz",popen.call_args.args[0])
                first=json.loads((root/"admin.json").read_text())
                self.assertGreaterEqual(len(first["password"]),43)
                arguments=run.call_args.args[0]
                self.assertIn("-admin-password-stdin",arguments)
                self.assertNotIn(first["password"],str(arguments))
                self.assertEqual(run.call_args.kwargs["input"],first["password"]+"\n")
                (root/"postgres-data.marker").write_bytes(b"existing data")
                run.reset_mock();deploy(manifest)
                run.assert_not_called()
                self.assertEqual(json.loads((root/"admin.json").read_text()),first)
                self.assertEqual((root/"postgres-data.marker").read_bytes(),b"existing data")
                self.assertIn("-db-config",popen.call_args.args[0])
                self.assertNotIn("--db",popen.call_args.args[0])

    def test_manifest_validation_precedes_remote_mutation(self):
        manifest={"root":"/tmp/forbidden","release":"123-abcdef123456","sha256":"a"*64,"version":"fixture","addr":"127.0.0.1:9999","publicHost":"teleport.engineering"}
        with self.assertRaises(ValueError):validate_manifest(manifest)

    def test_public_aliases_are_validated_before_installation(self):
        manifest={"root":"/home/exedev/teleport","release":"123-abcdef123456","sha256":"a"*64,"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering","publicHosts":["teleport.engineering","teleport.exe.xyz"]}
        self.assertEqual(validate_manifest(manifest),Path("/home/exedev/teleport"))
        for aliases in ([],"teleport.exe.xyz",["teleport.exe.xyz"],["teleport.engineering","https://teleport.exe.xyz"],["teleport.engineering","bad.host."],["teleport.engineering","*.exe.xyz"]):
            with self.subTest(aliases=aliases),self.assertRaises(ValueError):
                validate_manifest(dict(manifest,publicHosts=aliases))

    def test_unsafe_log_cannot_stop_an_existing_server(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture")
            (root/"server.log").symlink_to(base/"unrelated")
            secure_json(root/"server.json",{"pid":12345})
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:9999","publicHost":"teleport.engineering"}
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.stop_owned") as stop:
                with self.assertRaises(RuntimeError):deploy(manifest)
                stop.assert_not_called()

    def test_existing_file_database_requires_verified_migration_before_stop(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture")
            (root/"database.db").write_bytes(b"retained installation")
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering"}
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.stop_owned") as stop:
                with self.assertRaisesRegex(RuntimeError,"migration bridge"):deploy(manifest)
                stop.assert_not_called()
                self.assertEqual((root/"database.db").read_bytes(),b"retained installation")

    def test_failed_release_restores_previous_process_and_configuration(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture")
            old=root/"releases/100-abcdef123456";old.mkdir();(root/"current").symlink_to(old)
            secure_json(root/"server.json",{"pid":100,"version":"old","release":str(old)})
            secure_json(root/"admin.json",{"username":"admin","password":"x"*43,"bootstrapComplete":True})
            config=root/"database.json";secure_json(config,{"url":"postgresql:///fixture"})
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering"}
            failed=MagicMock(pid=200);failed.poll.return_value=1
            restored=MagicMock(pid=300)
            previous=[str(old/"tk"),"serve","--db-config",str(config)]
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.prepare_database",return_value=(config,False)),patch("deploy_remote.process_arguments",return_value=previous),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.subprocess.Popen",side_effect=[failed,restored]) as popen,patch("deploy_remote.stop_owned"):
                with self.assertRaisesRegex(RuntimeError,"exited"):deploy(manifest)
                self.assertEqual(popen.call_args.args[0],previous)
                self.assertEqual((root/"current").resolve(),old)
                self.assertEqual(json.loads((root/"server.json").read_text())["pid"],300)
                self.assertEqual(json.loads(config.read_text())["url"],"postgresql:///fixture")

    def test_first_run_failure_retains_initialized_target_for_retry(self):
        with tempfile.TemporaryDirectory() as temporary:
            base=Path(temporary).resolve();root=base/"teleport";release=root/"releases/123-abcdef123456";release.mkdir(parents=True)
            binary=release/"tk";binary.write_bytes(b"fixture")
            config=root/"database.json"
            def prepare(root):
                existed=config.exists()
                if not existed:secure_json(config,{"url":"postgresql:///same-target"})
                return config,not existed
            manifest={"root":str(root),"release":release.name,"sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"version":"fixture","addr":"127.0.0.1:8000","publicHost":"teleport.engineering"}
            failed=MagicMock(pid=200);failed.poll.return_value=1
            healthy=MagicMock(pid=300);healthy.poll.return_value=None
            class Response:
                def __enter__(self):return self
                def __exit__(self,*args):pass
                def read(self):return b'{"version":"fixture"}'
            with patch("deploy_remote.HOME_BASE",base),patch("deploy_remote.prepare_database",side_effect=prepare),patch("deploy_remote.subprocess.check_output",return_value="fixture"),patch("deploy_remote.subprocess.run") as run,patch("deploy_remote.subprocess.Popen",side_effect=[failed,healthy]),patch("deploy_remote.urllib.request.urlopen",return_value=Response()),patch("deploy_remote.stop_owned"):
                with self.assertRaisesRegex(RuntimeError,"exited"):deploy(manifest)
                self.assertTrue(config.is_file())
                self.assertTrue(json.loads((root/"admin.json").read_text())["bootstrapComplete"])
                run.reset_mock();deploy(manifest)
                run.assert_not_called()
                self.assertEqual(json.loads(config.read_text())["url"],"postgresql:///same-target")


if __name__=="__main__":
    from tool_history import run
    run(unittest.main)
