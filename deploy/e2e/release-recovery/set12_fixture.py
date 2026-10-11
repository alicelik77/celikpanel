#!/usr/bin/env python3
"""set12: owner_update_trial with one more published baseline, v0.1.0-alpha.82, for the twelfth native record.

Nothing of owner_update_trial.py or worker_fixture_origin.py is changed on disk. Importing this module (or running it
as the build's fixture driver) adds, in this process only:

* ``BASELINE_REFS["v0.1.0-alpha.82"]``: the baseline is the published tag's own commit (2a0af886; like alpha.81 it
  already carries the D-027 acceptance-licence seam and its guard, so no file is patched); the candidates are the
  source labelled ``v0.1.0-alpha.83 / 83`` (previous = the tag commit).
* the fixture origin's closed set of target transitions gains ``v0.1.0-alpha.83 / 83, previous 82``: a copy of
  worker_fixture_origin.py with exactly that tuple appended is written under ``$SET12_RUN_ROOT/generated/`` (its
  ``HERE`` names this directory, so its helper modules are this directory's) and is the module the driver loads and
  uploads to the guest as ``worker-fixture-origin.py``. The copy's SHA-256 and the original's are recorded beside it.
* for the alpha.82 baseline, ``LEDGER_AFTER_RETURN = 43`` (alpha.82's migration ledger; the candidate's is 44).

  set12_fixture.py fixture-source ...   (build-set12-artifacts.sh; the same arguments as owner_update_trial.py)
  set12_fixture.py prove --artifacts A.json

set12: yayımlanmış v0.1.0-alpha.82 tabanını ve alpha.83 adayını yalnız bu süreçte tanıtan ince katman; hiçbir
dosyayı diskte değiştirmez.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import owner_update_trial as out  # noqa: E402

ALPHA82_COMMIT = "2a0af88660773b43cbe1155c622483e2f0a2bbeb"
ALPHA82_REF = "v0.1.0-alpha.82"
CANDIDATE_POLICY_83 = {"version": "v0.1.0-alpha.83", "current": 83, "previous": 82, "previous_version": "v0.1.0-alpha.82"}
ORIGIN_NAME = "upd1_worker_fixture_origin"
RUN_ROOT = Path(os.environ.get("SET12_RUN_ROOT", "/var/tmp/cp-set12-run"))
HERE_LINE = "HERE = Path(__file__).resolve().parent\n"
POLICIES_ANCHOR = "\n\ndef allowed_policy(policy):"


def origin_text(original: str) -> str:
    """The origin's text with HERE pinned to this directory and the alpha.83 transition appended (exact edits)."""
    if original.count(HERE_LINE) != 1 or original.count(POLICIES_ANCHOR) != 1:
        raise ValueError("worker_fixture_origin.py differs from the reviewed text; the set12 copy is not made")
    text = original.replace(HERE_LINE, f"HERE = Path({str(HERE)!r})\n")
    added = ("\n# set12 (generated, not a repository file): the update from the published v0.1.0-alpha.82 to a "
             "candidate labelled\n# v0.1.0-alpha.83 is one more closed transition.\n"
             f"RELEASE_POLICIES = RELEASE_POLICIES + ({CANDIDATE_POLICY_83!r},)\n")
    return text.replace(POLICIES_ANCHOR, "\n" + added + POLICIES_ANCHOR)


def install() -> dict:
    out.BASELINE_REFS.setdefault(ALPHA82_REF, {
        "commit": ALPHA82_COMMIT, "baseline": (ALPHA82_REF, 82), "candidate": ("v0.1.0-alpha.83", 83),
        "unpatched": True,
        "baseline_policy": {"version": ALPHA82_REF, "current": 82, "previous": 81, "previous_version": "v0.1.0-alpha.81"}})
    if ORIGIN_NAME in out._MODULES:
        return {"origin": "already loaded"}
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    generated_dir = RUN_ROOT / "generated"
    generated_dir.mkdir(parents=True, exist_ok=True)
    target = generated_dir / "worker_fixture_origin.py"
    text = origin_text(original)
    if not target.exists() or target.read_text() != text:
        target.write_text(text)
    record = {"original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
              "generated": str(target), "generated_sha256": hashlib.sha256(text.encode()).hexdigest(),
              "added_policy": CANDIDATE_POLICY_83}
    (generated_dir / "worker_fixture_origin.json").write_text(json.dumps(record, indent=2, sort_keys=True) + "\n")
    spec = importlib.util.spec_from_file_location(ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[ORIGIN_NAME] = module
    return record


def configure(document: dict) -> dict:
    """configure_labels for a document; with the alpha.82 baseline the return's ledger is 43."""
    labels = out.configure_labels(document)
    if labels.get("ref") == ALPHA82_REF:
        out.LEDGER_AFTER_RETURN = 43
    return labels


PAIRS_LINE = "    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'))\n"
PAIRS_NEW = ("    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'),"
             " ('v0.1.0-alpha.82', 'v0.1.0-alpha.83'))  # set12 (generated copy)\n")


def bound_worker_copy() -> Path:
    """guest_bound_worker.py with the (alpha.82, alpha.83) pair added to its closed set of baseline/target labels, written
    under ``$SET12_RUN_ROOT/generated/`` and uploaded to the guest in place of the original (the owner-update observer
    validates its intent with it); its SHA-256 and the original's are recorded beside it. Nothing else differs."""
    source = HERE / "guest_bound_worker.py"
    original = source.read_text()
    if original.count(PAIRS_LINE) != 1:
        raise ValueError("guest_bound_worker.py differs from the reviewed text; the set12 copy is not made")
    text = original.replace(PAIRS_LINE, PAIRS_NEW)
    generated_dir = RUN_ROOT / "generated"
    generated_dir.mkdir(parents=True, exist_ok=True)
    target = generated_dir / "guest_bound_worker.py"
    if not target.exists() or target.read_text() != text:
        target.write_text(text)
    (generated_dir / "guest_bound_worker.json").write_text(json.dumps({
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "generated": str(target), "generated_sha256": hashlib.sha256(text.encode()).hexdigest(),
        "added_pair": ["v0.1.0-alpha.82", "v0.1.0-alpha.83"]}, indent=2, sort_keys=True) + "\n")
    return target


install()
baseline_ref_patched = out.baseline_ref_patched


if __name__ == "__main__":
    raise SystemExit(out.main())
