"""Offline tests of set14b's pure rules (no guest).

set14b_trial imports set14_trial and so set12_trial, whose set12_fixture changes owner_update_trial's module state and
writes its generated origin under $SET12_RUN_ROOT at import: as test_set14_trial does, the import happens inside the
class with SET12_RUN_ROOT pointing at a temporary directory, and that state is put back afterwards."""
from __future__ import annotations

import importlib
import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402

t = None  # set14b_trial, imported in setUpClass


def guest():
    spec = importlib.util.spec_from_file_location("guest_set14b_native_under_test", HERE / "guest_set14b_native.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class Set14bRules(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), out.LEDGER_AFTER_RETURN,
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"))
        cls.tmp = tempfile.TemporaryDirectory(prefix="set14b-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        global t
        t = importlib.import_module("set14b_trial")

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

    def test_cell_plan_and_set14_names(self):
        self.assertEqual(sorted(t.CELLS), ["set14b-debian13"])
        self.assertEqual(t.CELLS["set14b-debian13"].platform, "debian13-arch")
        self.assertEqual(t.BROWSER["set14b-debian13"]["lang"], "en")
        self.assertNotIn("set14b-debian13", t.s14.BROWSER)  # lent to set14's window only while it runs
        self.assertIsNot(t.s14.PREPS, t.PREPS)
        artifacts = {"baseline": {"version": "v0.1.0-alpha.83", "commit": "c" * 40, "sha256": "0" * 64}}
        plan = t.build_plan("set14b-debian13", artifacts, "/var/tmp/cp-release-drill-s14b-x", 19000)
        self.assertEqual(plan["steps"][-3:], ["set14-browser-window", t.s6.PIN_END_STEP, "collect"])
        self.assertFalse(plan["native_evidence"])

    def test_prep_names_and_shift(self):
        for name in t.PREPS:
            self.assertEqual(t.prep_name(f"{name}-{int(time.time() * 1000)}"), name)
        self.assertEqual(t.prep_name("clockshift_p1734-1791839755193"), "clockshift_p1734")
        self.assertEqual(t.shift_of("clockshift_m420000"), -420000)
        self.assertEqual(t.shift_of("clockshift_p1734"), 1734)
        for bad in ("clockshift_p0", "clockshift_m900001", "clockshift_x5", "clockshift_p-5", "probe"):
            self.assertIsNone(t.shift_of(bad), bad)

    def test_scan_fields(self):
        self.assertEqual(t.scan_fields(None), {"json_object": False})
        value = t.scan_fields({"scanned_at": "2026-10-13T06:00:00Z", "scan_age_seconds": 1.5, "services": [1, 2]})
        self.assertEqual((value["scan_age_seconds"], value["scan_age_seconds_type"], value["services"]), (1.5, "float", 2))
        self.assertEqual(t.scan_fields({"scanned_at": None, "scan_age_seconds": None})["scan_age_seconds_type"], "NoneType")
        self.assertEqual(t.scan_fields({"code": "AGENT_UNREACHABLE"})["scan_age_seconds_type"], "absent")

    def test_guest_refuses_bad_shift_and_keeps_modes(self):
        g = guest()
        for bad in ({}, {"shift_ms": 0}, {"shift_ms": 900001}, {"shift_ms": True}, {"shift_ms": 1.5}):
            with self.assertRaises(g.Refused):
                g.lab_clock14b(bad)
        for mode in ("read-probe14b", "lab-clock14b", "read-probe14", "owner-agent14"):
            self.assertIn(mode, g.MODES)


if __name__ == "__main__":
    unittest.main()
