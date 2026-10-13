"""Offline tests of set14c's pure rules (no guest).

set14c_trial imports set14b_trial and so set14_trial and so set12_trial, whose set12_fixture changes owner_update_trial's module state and
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

t = None  # set14c_trial, imported in setUpClass


class Set14cRules(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), out.LEDGER_AFTER_RETURN,
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"))
        cls.tmp = tempfile.TemporaryDirectory(prefix="set14c-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        global t
        t = importlib.import_module("set14c_trial")

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

    def test_cell_plan_and_names_lent_only_while_the_window_runs(self):
        self.assertEqual(sorted(t.CELLS), ["set14c-debian13"])
        self.assertEqual(t.CELLS["set14c-debian13"].platform, "debian13-arch")
        self.assertEqual(t.BROWSER["set14c-debian13"]["viewport"], "desktop")
        self.assertNotIn("set14c-debian13", t.s14b.BROWSER)
        self.assertNotIn("set14c-debian13", t.s14.BROWSER)
        self.assertIsNot(t.s14b.PREPS, t.PREPS)
        self.assertIn("agent-reads", t.PREPS)
        artifacts = {"baseline": {"version": "v0.1.0-alpha.83", "commit": "c" * 40, "sha256": "0" * 64}}
        plan = t.build_plan("set14c-debian13", artifacts, "/var/tmp/cp-release-drill-s14c-x", 19000)
        self.assertEqual(plan["schema"], "celikpanel/set14c-plan/v1")
        self.assertEqual(plan["steps"][-3:], ["set14-browser-window", t.s6.PIN_END_STEP, "collect"])
        self.assertFalse(plan["native_evidence"])

    def test_prep_names_and_shifts(self):
        for name in t.PREPS:
            self.assertEqual(t.prep_name(f"{name}-{int(time.time() * 1000)}"), name)
        self.assertEqual(t.shift_of("clockshift_p840000"), 840000)
        self.assertEqual(t.shift_of("clockshift_m420000"), -420000)
        self.assertIsNone(t.shift_of("clockshift_p900001"))

    def test_read_fields(self):
        sentence = "The CelikPanel Agent on this server is not reachable, so this request was not carried out."
        value = t.read_fields(503, {"code": "AGENT_UNREACHABLE", "error": sentence}, None)
        self.assertEqual((value["status"], value["code"], value["error"], value["keys"]), (503, "AGENT_UNREACHABLE", sentence, ["code", "error"]))
        self.assertFalse(value["body_mentions_socket"])
        self.assertTrue(t.read_fields(500, {"error": "dial unix /run/celikpanel/agent.sock: connect: refused"}, None)["body_mentions_socket"])
        plain = t.read_fields(500, None, "internal error")
        self.assertEqual((plain["keys"], plain["body"]), (None, "internal error"))


if __name__ == "__main__":
    unittest.main()
