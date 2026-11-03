"""Verify database command privacy, deadlines and external-cluster ownership."""
from pathlib import Path
import subprocess
import time
import unittest
from unittest.mock import patch

from postgres import _deadline,cluster,pg


class PostgresToolTest(unittest.TestCase):
    def test_administration_passwords_never_enter_process_arguments(self):
        for url,password in (("postgresql://alice:private%40password@example.test/postgres","private@password"),("postgresql://alice:private@password@example.test/postgres","private@password"),("postgresql://alice@example.test/postgres?password=private-secret","private-secret")):
            with self.subTest(url=url),patch("postgres.subprocess.run",return_value=subprocess.CompletedProcess([],0,"","")) as run:
                pg(Path("/tools"),"createdb","--maintenance-db",url,"fixture")
                self.assertNotIn(password,str(run.call_args.args[0]))
                self.assertNotIn("password",str(run.call_args.args[0]))
                self.assertEqual(run.call_args.kwargs["env"]["PGPASSWORD"],password)

    def test_owned_database_commands_share_the_supervisors_remaining_budget(self):
        token=_deadline.set(time.monotonic()+.2)
        try:
            with patch("postgres.subprocess.run",return_value=subprocess.CompletedProcess([],0,"","")) as run:
                pg(Path("/tools"),"pg_ctl","status")
                self.assertGreater(run.call_args.kwargs["timeout"],0)
                self.assertLessEqual(run.call_args.kwargs["timeout"],.2)
        finally:_deadline.reset(token)

    def test_explicit_test_cluster_is_borrowed_without_start_or_shutdown(self):
        environment={"TELEPORT_TEST_POSTGRES":"postgresql:///isolated","fixture":"unchanged"}
        with patch("postgres.binaries",side_effect=AssertionError("must not provision")):
            with cluster(environment) as checked:
                self.assertEqual(checked,environment)
                checked["fixture"]="local"
        self.assertEqual(environment["fixture"],"unchanged")


if __name__=="__main__":
    from tool_history import run
    run(unittest.main)
