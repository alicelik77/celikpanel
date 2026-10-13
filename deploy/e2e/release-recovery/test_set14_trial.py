"""Offline tests of set14's pure rules (no guest).

set14_trial imports set13_trial and so set12_trial, whose set12_fixture changes owner_update_trial's module state and
writes its generated origin under $SET12_RUN_ROOT at import: as test_set13_trial does, the import happens inside the
class with SET12_RUN_ROOT pointing at a temporary directory, and that state is put back afterwards."""
from __future__ import annotations

import importlib
import importlib.util
import os
from pathlib import Path
import sqlite3
import sys
import tempfile
import time
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402

t = None  # set14_trial, imported in setUpClass


def guest():
    spec = importlib.util.spec_from_file_location("guest_set14_native_under_test", HERE / "guest_set14_native.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class Set14Rules(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), out.LEDGER_AFTER_RETURN,
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"))
        cls.tmp = tempfile.TemporaryDirectory(prefix="set14-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        global t
        t = importlib.import_module("set14_trial")

    @classmethod
    def tearDownClass(cls):
        modules, refs, ledger, origin, root = cls.saved
        out._MODULES.clear()
        out._MODULES.update(modules)
        out.BASELINE_REFS.clear()
        out.BASELINE_REFS.update(refs)
        out.LEDGER_AFTER_RETURN = ledger
        if origin is None:
            sys.modules.pop("upd1_worker_fixture_origin", None)
        else:
            sys.modules["upd1_worker_fixture_origin"] = origin
        if root is None:
            os.environ.pop("SET12_RUN_ROOT", None)
        else:
            os.environ["SET12_RUN_ROOT"] = root
        cls.tmp.cleanup()

    def test_cells_browser_and_plan(self):
        self.assertEqual(sorted(t.CELLS), ["set14-arch", "set14-debian13", "set14-ubuntu"])
        self.assertEqual(sorted(t.BROWSER), sorted(t.CELLS))
        self.assertEqual({t.BROWSER["set14-debian13"]["lang"], t.BROWSER["set14-ubuntu"]["lang"]}, {"en", "tr"})
        for cell in t.CELLS.values():
            self.assertEqual(cell.purpose, "web")
            self.assertFalse(cell.mail)
        artifacts = {"baseline": {"version": "v0.1.0-alpha.81", "commit": "c" * 40, "sha256": "0" * 64}}
        plan = t.build_plan("set14-ubuntu", artifacts, "/var/tmp/cp-release-drill-s14-x", 19000)
        self.assertEqual(plan["steps"][-3:], ["set14-browser-window", t.s6.PIN_END_STEP, "collect"])
        self.assertEqual(plan["steps"][0], t.s6.PIN_STEP)
        self.assertEqual(plan["browser"], {"lang": "tr", "viewport": "phone"})
        self.assertFalse(plan["native_evidence"])

    def test_prep_name_and_compact_probe(self):
        self.assertEqual(t.prep_name("agent-stop-1791838458775"), "agent-stop")
        self.assertEqual(t.prep_name("probe-1"), "probe")
        for name in t.PREPS:
            self.assertEqual(t.prep_name(f"{name}-{int(time.time() * 1000)}"), name)
        value = {"at": "x", "epoch": 1.0, "scan_cache": {"exists": False},
                 "units": {"agent": {"ActiveState": "active", "MainPID": "12", "ControlGroup": "/system.slice/a"}},
                 "cgroup_cpu": {"agent": {"usage_usec": 5}}, "nginx_dropin": {"exists": True, "text": "secret?"}}
        brief = t.compact_probe(value)
        self.assertEqual(brief["units"]["agent"], {"ActiveState": "active", "MainPID": "12", "NRestarts": None, "CPUUsageNSec": None})
        self.assertEqual(brief["units"]["panel"]["ActiveState"], None)
        self.assertIs(brief["nginx_dropin"], True)

    def test_guest_scan_cache_reads_row_and_age(self):
        g = guest()
        with tempfile.TemporaryDirectory(prefix="set14-db-") as tmp:
            db = os.path.join(tmp, "celikpanel.db")
            saved = g.g10.PANEL_DB
            g.g10.PANEL_DB = db
            try:
                connection = sqlite3.connect(db)
                connection.execute("CREATE TABLE service_scan_cache (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL, scanned_at TEXT NOT NULL)")
                connection.commit()
                self.assertEqual(g.scan_cache()["exists"], False)
                stamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 400))
                connection.execute("INSERT INTO service_scan_cache VALUES (1, ?, ?)", ("{}" * 10, stamp))
                connection.commit()
                connection.close()
                value = g.scan_cache()
                self.assertTrue(value["exists"])
                self.assertEqual(value["scanned_at"], stamp)
                self.assertEqual(value["data_bytes"], 20)
                self.assertTrue(395 <= value["age_seconds"] <= 410, value["age_seconds"])
            finally:
                g.g10.PANEL_DB = saved

    def test_guest_proc_cpu_and_cgroup_parsers(self):
        g = guest()
        self.assertEqual(g.cpu_stat("")["error"], "no control group")
        self.assertIn("error", g.cpu_stat("/no/such/group-set14"))
        self.assertIn("error", g.proc_cpu("not-a-pid"))
        if os.path.exists("/proc/self/stat"):
            value = g.proc_cpu(str(os.getpid()))
            self.assertEqual(sorted(value), ["clk_tck", "cstime", "cutime", "stime", "utime"])

    def test_guest_refuses_bad_arguments(self):
        g = guest()
        with self.assertRaises(g.Refused):
            g.owner_agent14({"action": "restart"})
        with self.assertRaises(g.Refused):
            g.lab_slow_reload14({"seconds": 1})
        self.assertIn("read-probe14", g.MODES)
        self.assertIn("read-unit13", g.MODES)  # set13's modes are kept


if __name__ == "__main__":
    unittest.main()
