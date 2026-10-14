"""Offline tests of set16_fixture's profiles and generated copies, set16_trial's plan pieces and the browser tool's
preparation names (no guest, no network). Run with ``python3 -I -m unittest`` from this directory's parent, or
``cd deploy/e2e/release-recovery && python3 -I -m unittest test_set16_trial``.

set16_fixture (like set15_fixture) changes owner_update_trial's module state in its own process; these tests import it
only inside their class and put that state back afterwards, so the other offline tests see owner_update_trial as it is.
"""
from __future__ import annotations

import importlib
import os
from pathlib import Path
import re
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402

LABEL_GLOBALS = ("BASELINE_VERSION", "BASELINE_SEQUENCE", "CANDIDATE_VERSION", "CANDIDATE_SEQUENCE", "LABEL_REF",
                 "LEDGER_AFTER_UPDATE", "LEDGER_AFTER_RETURN", "provenance_for")
FIXTURES = ("set12_fixture", "set15_fixture", "set16_fixture")
TOOL = HERE.parents[2] / "web" / "tools" / "browser-inspect" / "cold-load-set16.mjs"


class Set16Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import set6_trial as s6
        cls.s6 = s6
        cls.saved = (dict(out._MODULES), {k: dict(v) for k, v in out.BASELINE_REFS.items()},
                     {k: getattr(out, k) for k in LABEL_GLOBALS}, sys.modules.get("upd1_worker_fixture_origin"),
                     os.environ.get("SET12_RUN_ROOT"), s6.HSTS_ALPHA81)
        cls.tmp = tempfile.TemporaryDirectory(prefix="set16-test-")
        os.environ["SET12_RUN_ROOT"] = cls.tmp.name
        out._MODULES.pop("upd1_worker_fixture_origin", None)
        for name in FIXTURES:
            sys.modules.pop(name, None)
        cls.fx = importlib.import_module("set16_fixture")
        cls.t = importlib.import_module("set16_trial")

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
        for name in FIXTURES:
            sys.modules.pop(name, None)
        if origin is None:
            sys.modules.pop("upd1_worker_fixture_origin", None)
        else:
            sys.modules["upd1_worker_fixture_origin"] = origin
        if root is None:
            os.environ.pop("SET12_RUN_ROOT", None)
        else:
            os.environ["SET12_RUN_ROOT"] = root
        cls.tmp.cleanup()

    def fresh_document(self, commit="a" * 40, baseline=None):
        return {"baseline_ref": {"ref": "v0.1.0-alpha.85", "tag_commit": commit},
                "baseline": {"commit": baseline or commit}}

    def test_update_profile_replaces_set15_label(self):
        a84 = out.BASELINE_REFS["v0.1.0-alpha.84"]
        self.assertEqual((a84["commit"], a84["baseline"], a84["candidate"]),
                         ("f0157e6c8f1614cff176821172f8a9067493e625", ("v0.1.0-alpha.84", 84), ("v0.1.0-alpha.85", 85)))
        self.assertTrue(a84["unpatched"])
        fresh = out.BASELINE_REFS["v0.1.0-alpha.85"]
        self.assertEqual((fresh["baseline"], fresh["candidate"]), (("v0.1.0-alpha.85", 85), ("v0.1.0-alpha.86", 86)))
        self.assertEqual(fresh["baseline_policy"]["previous"], 84)
        self.assertIn("v0.1.0-alpha.83", out.BASELINE_REFS)

    def test_origin_copy_adds_86(self):
        origin = out._MODULES[self.fx.fx12.ORIGIN_NAME]
        self.assertEqual(list(origin.RELEASE_POLICIES[-4:]),
                         [self.fx.fx15.POLICY_83, self.fx.POLICY_84, self.fx.POLICY_85, self.fx.POLICY_86])
        for policy in (self.fx.POLICY_85, self.fx.POLICY_86):
            self.assertTrue(origin.allowed_policy(dict(policy)))
        self.assertFalse(origin.allowed_policy({"version": "v0.1.0-alpha.87", "current": 87, "previous": 86,
                                                "previous_version": "v0.1.0-alpha.86"}))
        self.assertIn("generated-set16", str(origin.__file__))

    def test_bound_worker_copy_adds_both_pairs(self):
        text = self.fx.bound_worker_copy().read_text()
        self.assertIn("('v0.1.0-alpha.83', 'v0.1.0-alpha.84')", text)
        self.assertIn("('v0.1.0-alpha.84', 'v0.1.0-alpha.85')", text)
        self.assertEqual(text.count("pairs = ("), 1)

    def test_configure_alpha84(self):
        labels = self.fx.configure({"baseline_ref": {"ref": "v0.1.0-alpha.84", "tag_commit": self.fx.ALPHA84_COMMIT}})
        self.assertEqual((labels["baseline"], labels["candidate"]), (("v0.1.0-alpha.84", 84), ("v0.1.0-alpha.85", 85)))
        self.assertEqual((out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN), (44, 44))
        self.assertEqual(self.s6.HSTS_ALPHA81, "max-age=31536000")
        text = out.provenance_for("good")
        self.assertIn("published-tag-v0.1.0-alpha.84", text["baseline"])
        self.assertNotIn("1636b73f0", text["baseline"])   # set15's label profile of the same name is gone
        self.assertIn("labelled-v0.1.0-alpha.85", text["candidate"])

    def test_configure_fresh_takes_the_installed_commit(self):
        labels = self.fx.configure(self.fresh_document("b" * 40))
        self.assertEqual(labels["ref"], "v0.1.0-alpha.85")
        self.assertEqual(out.BASELINE_REFS["v0.1.0-alpha.85"]["commit"], "b" * 40)
        self.assertNotIn("published-tag", out.provenance_for("good")["baseline"])
        with self.assertRaises(ValueError):
            self.fx.configure(self.fresh_document("c" * 40, baseline="d" * 40))
        with self.assertRaises(ValueError):
            self.fx.configure(self.fresh_document("not-a-commit"))

    def test_clock_preparations_parse(self):
        shift_of = self.t.s14b.shift_of
        names = [n for n in self.t.FRESH_PREPS if n.startswith("clockshift_")]
        self.assertEqual(sorted(shift_of(n) for n in names), [-180000, 90000])

    def test_browser_tool_asks_only_for_known_preparations(self):
        """Every prep('name') the set16 browser tool sends is one the cell's driver answers."""
        text = TOOL.read_text(encoding="utf-8")
        asked = set(re.findall(r"prep\('([a-z0-9_-]+)'", text))
        self.assertTrue({"probe", "journal", "clockshift_p90000", "clockshift_m180000", "ntp-on", "agent-stop",
                         "agent-start"} <= asked)
        self.assertTrue(asked <= set(self.t.FRESH_PREPS), asked - set(self.t.FRESH_PREPS))
        self.assertTrue({"probe", "journal"} <= set(self.t.UPDATE_PREPS))
        # the update window runs only the phases that ask for probe and journal
        self.assertIn("if (fresh) {", text)

    def test_ledger_summary(self):
        check = {"verdict": "equal-except-volatile", "unexpected": [],
                 "volatile_columns_only": {"managed_site_files": ["observed_at"]},
                 "site_file_ledger": {"full_equal": False, "stable_equal": True, "rows_before": 1, "rows_after": 1,
                                      "rows_added": [], "rows_removed": [],
                                      "changed_columns_by_rowid": {"1": ["observed_at"]}}}
        value = self.t.ledger_summary(check)
        self.assertTrue(value["only_observed_at_changed"])
        self.assertEqual(value["changed_columns"], ["observed_at"])
        check["site_file_ledger"]["changed_columns_by_rowid"] = {"1": ["observed_at", "state"]}
        self.assertFalse(self.t.ledger_summary(check)["only_observed_at_changed"])
        self.assertEqual(self.t.ledger_summary({"verdict": "different"})["site_file_ledger"], "not-recorded")

    def test_guest_helper_chain_is_uploaded(self):
        source = (HERE / "set16_trial.py").read_text()
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
        self.assertEqual(sorted(self.t.CELLS), ["set16-debian13", "set16-ubuntu"])
        self.assertEqual({c.purpose for c in self.t.CELLS.values()}, {"web"})
        self.assertTrue(set(self.t.UPDATE_CELLS) <= set(out.CELLS))
        self.assertTrue(set(self.t.CELLS) | set(self.t.UPDATE_CELLS) <= set(self.t.BROWSER))

    def test_build_script_labels_only_the_policy(self):
        text = (HERE / "build-set16-artifacts.sh").read_text()
        self.assertIn("TAG84=f0157e6c8f1614cff176821172f8a9067493e625", text)
        self.assertIn('only_policy_changed "$source_commit" "$good"', text)
        self.assertIn("expected_policy v0.1.0-alpha.85 85 84 v0.1.0-alpha.84", text)
        self.assertIn("set16_fixture.py", text)


if __name__ == "__main__":
    unittest.main()
