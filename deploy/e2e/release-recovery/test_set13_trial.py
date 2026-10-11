"""Offline tests of set13's pure rules (no guest).

set13_trial imports set12_trial, whose set12_fixture changes owner_update_trial's module state and writes its generated
origin under $SET12_RUN_ROOT at import: as test_set12_trial does, the import happens inside the class with
SET12_RUN_ROOT pointing at a temporary directory, and that state is put back afterwards."""
from __future__ import annotations

import importlib
import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402

t = None  # set13_trial, imported in setUpClass


def guest():
    spec = importlib.util.spec_from_file_location("guest_set13_native_under_test", HERE / "guest_set13_native.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class Set13Rules(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), out.LEDGER_AFTER_RETURN,
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"))
        cls.tmp = tempfile.TemporaryDirectory(prefix="set13-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        global t
        t = importlib.import_module("set13_trial")

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

    def test_sections_and_cells(self):
        self.assertEqual([k for k, _ in t.SECTIONS][:2], ["S0-sites", "h-i-unchanged-start"])
        self.assertEqual(sorted(t.CELLS), ["set13-arch", "set13-debian13", "set13-ubuntu"])
        self.assertIn("node -e", t.NODE_COMMAND)
        self.assertFalse(t.NODE_COMMAND.startswith("/"))

    def test_service_fields_keeps_only_the_answer_facts(self):
        body = {"code": "SITE_CONFIG_SERVICE_REFUSED", "restore": "confirmed", "service": "not_reloaded",
                "service_reason": "config_refused_elsewhere", "service_active": "true", "other": 1}
        self.assertNotIn("other", t.service_fields(body))
        self.assertEqual(t.service_fields(body)["service"], "not_reloaded")

    def test_lines_with(self):
        lines = ["2026-10-11T10:00:00.000000+0000 host systemd[1]: Reloading php8.4-fpm.service", "x Started y"]
        self.assertEqual(len(t.lines_with(lines, ("Reloading",))), 1)
        self.assertEqual(t.lines_with(lines, ("absent",)), [])

    def test_unit_lines_and_shims(self):
        g = guest()
        text = ("[Service]\nEnvironment=PORT=3001\nEnvironment=PATH=/opt/celikpanel/runtimes/node/22.20.0/bin:/usr/local/sbin\n"
                "ExecStart=/bin/sh -c 'node -e 1'\n[Install]\n")
        lines = g.unit_lines(text)
        self.assertEqual(lines["port"], "3001")
        self.assertEqual(lines["exec_start"], ["ExecStart=/bin/sh -c 'node -e 1'"])
        self.assertEqual(len(lines["environment_path"]), 1)
        shim = g.shim_text("systemctl", "/usr/bin/systemctl")
        self.assertTrue(shim.startswith("#!/bin/sh\n" + g.SHIM_MARK))
        self.assertIn('exec /usr/bin/systemctl "$@"', shim)
        self.assertIn(g.FAIL_FLAG, shim)
        analyze = g.shim_text("systemd-analyze", "/usr/bin/systemd-analyze")
        self.assertIn("rc=$?", analyze)


if __name__ == "__main__":
    unittest.main()
