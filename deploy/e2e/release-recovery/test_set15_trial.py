"""Offline tests of set15_fixture's profiles and generated copies and of set15_trial's plan (no guest, no network).

set15_fixture changes owner_update_trial's module state in its own process (two more profiles, the origin module it
loads, the provenance text, set6's expected header); these tests import it only inside their class and put that state
back afterwards, so the other offline tests see owner_update_trial as it is.
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

LABEL_GLOBALS = ("BASELINE_VERSION", "BASELINE_SEQUENCE", "CANDIDATE_VERSION", "CANDIDATE_SEQUENCE", "LABEL_REF",
                 "LEDGER_AFTER_UPDATE", "LEDGER_AFTER_RETURN", "provenance_for")


class Set15Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import set6_trial as s6
        cls.s6 = s6
        cls.saved = (dict(out._MODULES), dict(out.BASELINE_REFS), {k: getattr(out, k) for k in LABEL_GLOBALS},
                     sys.modules.get("upd1_worker_fixture_origin"), os.environ.get("SET12_RUN_ROOT"), s6.HSTS_ALPHA81)
        cls.tmp = tempfile.TemporaryDirectory(prefix="set15-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        out._MODULES.pop("upd1_worker_fixture_origin", None)
        for name in ("set12_fixture", "set15_fixture"):
            sys.modules.pop(name, None)
        cls.fx = importlib.import_module("set15_fixture")
        cls.t = importlib.import_module("set15_trial")

    @classmethod
    def tearDownClass(cls):
        modules, refs, labels, origin, root, hsts = cls.saved
        out.configure_labels(None)
        out._MODULES.clear()
        out._MODULES.update(modules)
        out.BASELINE_REFS.clear()
        out.BASELINE_REFS.update(refs)
        for key, value in labels.items():
            setattr(out, key, value)
        cls.s6.HSTS_ALPHA81 = hsts
        for name in ("set12_fixture", "set15_fixture"):
            sys.modules.pop(name, None)   # a later test module imports set12_fixture afresh (its install runs again)
        if origin is None:
            sys.modules.pop("upd1_worker_fixture_origin", None)
        else:
            sys.modules["upd1_worker_fixture_origin"] = origin
        if root is None:
            os.environ.pop("SET12_RUN_ROOT", None)
        else:
            os.environ["SET12_RUN_ROOT"] = root
        cls.tmp.cleanup()

    def test_profiles_are_added(self):
        a83 = out.BASELINE_REFS["v0.1.0-alpha.83"]
        self.assertEqual((a83["commit"], a83["baseline"], a83["candidate"]),
                         (self.fx.ALPHA83_COMMIT, ("v0.1.0-alpha.83", 83), ("v0.1.0-alpha.84", 84)))
        fresh = out.BASELINE_REFS["v0.1.0-alpha.84"]
        self.assertEqual((fresh["commit"], fresh["baseline"], fresh["candidate"]),
                         (self.fx.SOURCE_COMMIT, ("v0.1.0-alpha.84", 84), ("v0.1.0-alpha.85", 85)))
        self.assertTrue(a83["unpatched"] and fresh["unpatched"])
        self.assertIn("v0.1.0-alpha.82", out.BASELINE_REFS)

    def test_origin_copy_adds_the_three_transitions(self):
        origin = out._MODULES[self.fx.fx12.ORIGIN_NAME]
        self.assertEqual(list(origin.RELEASE_POLICIES[-3:]), [self.fx.POLICY_83, self.fx.POLICY_84, self.fx.POLICY_85])
        for policy in (self.fx.POLICY_84, self.fx.POLICY_85):
            self.assertTrue(origin.allowed_policy(dict(policy)))
        self.assertFalse(origin.allowed_policy({"version": "v0.1.0-alpha.86", "current": 86, "previous": 85,
                                                "previous_version": "v0.1.0-alpha.85"}))
        self.assertTrue(str(Path(origin.__file__)).startswith(self.tmp.name))

    def test_bound_worker_copy_adds_the_pair(self):
        text = self.fx.bound_worker_copy().read_text()
        self.assertIn("('v0.1.0-alpha.83', 'v0.1.0-alpha.84')", text)
        self.assertEqual(text.count("pairs = ("), 1)

    def test_configure_alpha83(self):
        labels = self.fx.configure({"baseline_ref": {"ref": "v0.1.0-alpha.83", "tag_commit": self.fx.ALPHA83_COMMIT}})
        self.assertEqual(labels["candidate"], ("v0.1.0-alpha.84", 84))
        self.assertEqual((out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN), (44, 44))
        self.assertEqual(self.s6.HSTS_ALPHA81, "max-age=31536000")

    def test_fresh_provenance_names_the_source(self):
        self.fx.configure({"baseline_ref": {"ref": "v0.1.0-alpha.84", "tag_commit": self.fx.SOURCE_COMMIT}})
        text = out.provenance_for("good")["baseline"]
        self.assertIn(self.fx.SOURCE_COMMIT, text)
        self.assertNotIn("published-tag", text)

    def test_brief_body(self):
        body = {"scanned_at": "x", "scan_age_seconds": 12, "services": [1, 2, 3]}
        self.assertEqual(self.t.brief_body("/api/v1/managed-services", body),
                         {"scanned_at": "x", "scan_age_seconds": 12, "services_count": 3,
                          "keys": ["scan_age_seconds", "scanned_at", "services"]})
        self.assertEqual(self.t.brief_body("/api/v1/domains", [])["rows"], 0)
        self.assertEqual(self.t.brief_body("/api/v1/panel/version", {"version": "v"}), {"version": "v"})

    def test_guest_helper_chain_is_uploaded(self):
        """guest_set14_native.py loads guest_set13 ... guest_set4 and guest_request_identity on the guest (set15's update
        cell stopped at its first probe: run a without guest_set13_native.py, run b without guest_set4_native.py, which
        set5 uploads only with its item 9 or M10)."""
        import re
        source = (HERE / "set15_trial.py").read_text()
        chain, name = [], "guest_set14_native.py"
        while True:
            found = re.findall(r'_load\("(guest_[a-z0-9_]+\.py)"\)', (HERE / name).read_text())
            if not found:
                break
            name = found[0]
            chain.append(name)
        self.assertEqual(chain[-1], "guest_request_identity_native.py")
        for name in chain:
            self.assertIn(f'"{name}"', source)

    def test_cells(self):
        self.assertEqual(sorted(self.t.CELLS), ["set15-arch", "set15-debian13", "set15-ubuntu"])
        self.assertEqual({c.purpose for c in self.t.CELLS.values()}, {"web"})
        self.assertTrue(set(self.t.UPDATE_CELLS) <= set(out.CELLS))
        self.assertTrue(set(self.t.CELLS) | set(self.t.UPDATE_CELLS) <= set(self.t.BROWSER))


if __name__ == "__main__":
    unittest.main()
