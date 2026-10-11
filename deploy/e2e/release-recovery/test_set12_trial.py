"""Offline tests of set12's pure rules and of set12_fixture's generated origin (no guest, no network).

set12_fixture changes owner_update_trial's module state in its own process (one more baseline profile, the origin
module it loads); these tests import it only inside their class and put that state back afterwards, so the other
offline tests see owner_update_trial as it is.
"""
from __future__ import annotations

import importlib
import os
from pathlib import Path
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402


class Set12Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), out.LEDGER_AFTER_RETURN,
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"))
        cls.tmp = tempfile.TemporaryDirectory(prefix="set12-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        out._MODULES.pop("upd1_worker_fixture_origin", None)
        cls.fx = importlib.import_module("set12_fixture")
        cls.t = importlib.import_module("set12_trial")

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

    def test_alpha82_profile_is_added(self):
        profile = out.BASELINE_REFS["v0.1.0-alpha.82"]
        self.assertEqual(profile["candidate"], ("v0.1.0-alpha.83", 83))
        self.assertTrue(profile["unpatched"])
        self.assertEqual(self.fx.baseline_ref_patched("v0.1.0-alpha.82"), ())

    def test_origin_copy_adds_exactly_one_transition(self):
        origin = out._MODULES[self.fx.ORIGIN_NAME]
        self.assertEqual(origin.RELEASE_POLICIES[-1], self.fx.CANDIDATE_POLICY_83)
        self.assertTrue(origin.allowed_policy(dict(self.fx.CANDIDATE_POLICY_83)))
        self.assertEqual(len(origin.RELEASE_POLICIES), 3)

    def test_origin_text_refuses_other_text(self):
        with self.assertRaises(ValueError):
            self.fx.origin_text("RELEASE_POLICY = {}\n")

    def test_file_of(self):
        body = {"files": [{"kind": "nginx_vhost", "state": "managed_unchanged"}, {"kind": "php_pool", "state": "unknown"}]}
        self.assertEqual(self.t.file_of(body, "php_pool")["state"], "unknown")
        self.assertEqual(self.t.file_of(body, "app_unit"), {})

    def test_syn_destinations(self):
        rows = [{"syn": True, "sport": 5, "dport": 80, "dst": "::1"}, {"syn": True, "sport": 6, "dport": 443, "dst": "127.0.0.1"}]
        self.assertEqual(self.t.syn_destinations(rows), {5: "::1"})

    def test_first_state_line(self):
        rows = {"a": {"state": "managed_unchanged", "adopted_from": "v0.1.0-alpha.82"},
                "b": {"state": "unknown_origin"}, "c": {"state": "unknown_origin", "kept_by_choice": True}, "d": None}
        self.assertEqual(self.t.first_state_line(rows), {"adopted": 1, "left": 2, "drawn": True})
        self.assertFalse(self.t.first_state_line({"c": {"state": "unknown_origin", "kept_by_choice": True}})["drawn"])

    def test_sealed_ini(self):
        self.assertTrue(self.t.sealed_ini("; celikpanel-render v2 sha256=" + "0" * 64 + "\n[site1]\n"))
        self.assertFalse(self.t.sealed_ini("[site1]\n"))


if __name__ == "__main__":
    unittest.main()
