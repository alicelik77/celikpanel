#!/usr/bin/env python3
"""set15: owner_update_trial with two more release profiles, for the fifteenth native record (the pre-publication
measurement of v0.1.0-alpha.84, commit 1636b73f0).

Nothing of owner_update_trial.py, worker_fixture_origin.py or guest_bound_worker.py is changed on disk. Importing
this module (or running it as the build's fixture driver) first imports set12_fixture (alpha.82), then adds, in this
process only:

* ``BASELINE_REFS["v0.1.0-alpha.83"]``: the update cells' baseline is the alpha.83 tag's own commit (0447558e; it
  carries the D-027 acceptance-licence seam and its guard, so no file is patched); the candidate is labelled
  ``v0.1.0-alpha.84 / 84`` (previous = the tag commit), which is the source commit's own release policy.
* ``BASELINE_REFS["v0.1.0-alpha.84"]``: a label profile for the fresh-install cells. Their "baseline" (the build
  that is installed) is the unpublished source commit 1636b73f0 itself, labelled by its own policy; the origin's
  candidate (never installed in a fresh cell) is a fixture commit labelled ``v0.1.0-alpha.85 / 85``. The word
  "published-tag" in the inherited provenance text does not apply to this profile; ``provenance_for`` is wrapped to say so.
* the fixture origin's closed set of target transitions gains 84 (previous 83) and 85 (previous 84): a copy of
  worker_fixture_origin.py with exactly those tuples (and set12's 83) appended is written under
  ``$SET12_RUN_ROOT/generated-set15/`` and replaces set12's copy as the module the driver loads and uploads.
* ``bound_worker_copy()``: guest_bound_worker.py with the (alpha.83, alpha.84) pair added, uploaded in place of the
  original in the update cells (the owner-update observer validates its intent with it).
* for the alpha.83 baseline, ``LEDGER_AFTER_UPDATE = LEDGER_AFTER_RETURN = 44`` (alpha.83's migration ledger is 44;
  the candidate adds no migration), and set6's expected HSTS header before the update and after a return is
  alpha.83's own (``cmd/panel/security.go:46`` of the tag: ``max-age=31536000``).

  set15_fixture.py fixture-source ...   (build-set15-artifacts.sh; the same arguments as owner_update_trial.py)

set15: alpha.83 tabanını, alpha.84 adayını ve taze kurulum için alpha.84 etiket profilini yalnız bu süreçte tanıtan
ince katman; hiçbir dosyayı diskte değiştirmez.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set12_fixture as fx12  # noqa: E402  (registers alpha.82 and loads set12's origin copy first)

out = fx12.out
ALPHA83_COMMIT = "0447558e956d4c02c7bf06b368def0924856f8fc"
ALPHA83_REF = "v0.1.0-alpha.83"
SOURCE_COMMIT = "1636b73f0979d9921ff99f283ceec8cfed1041dc"
FRESH_REF = "v0.1.0-alpha.84"
POLICY_83 = dict(fx12.CANDIDATE_POLICY_83)
POLICY_84 = {"version": "v0.1.0-alpha.84", "current": 84, "previous": 83, "previous_version": "v0.1.0-alpha.83"}
POLICY_85 = {"version": "v0.1.0-alpha.85", "current": 85, "previous": 84, "previous_version": "v0.1.0-alpha.84"}
HSTS_ALPHA83 = "max-age=31536000"   # cmd/panel/security.go:46 at v0.1.0-alpha.83 (0447558e)
GENERATED = fx12.RUN_ROOT / "generated-set15"
PAIRS_NEW = ("    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'),"
             " ('v0.1.0-alpha.83', 'v0.1.0-alpha.84'))  # set15 (generated copy)\n")


def origin_text(original: str) -> str:
    """The origin's text with HERE pinned to this directory and the 83, 84 and 85 transitions appended (exact edits)."""
    if original.count(fx12.HERE_LINE) != 1 or original.count(fx12.POLICIES_ANCHOR) != 1:
        raise ValueError("worker_fixture_origin.py differs from the reviewed text; the set15 copy is not made")
    text = original.replace(fx12.HERE_LINE, f"HERE = Path({str(HERE)!r})\n")
    added = ("\n# set15 (generated, not a repository file): set12's alpha.83 transition, the update from alpha.83 to a "
             "candidate\n# labelled v0.1.0-alpha.84, and the fresh cells' origin candidate labelled v0.1.0-alpha.85.\n"
             f"RELEASE_POLICIES = RELEASE_POLICIES + ({POLICY_83!r}, {POLICY_84!r}, {POLICY_85!r})\n")
    return text.replace(fx12.POLICIES_ANCHOR, "\n" + added + fx12.POLICIES_ANCHOR)


def _write(name: str, text: str, record: dict) -> Path:
    GENERATED.mkdir(parents=True, exist_ok=True)
    target = GENERATED / name
    if not target.exists() or target.read_text() != text:
        target.write_text(text)
    (GENERATED / (Path(name).stem + ".json")).write_text(json.dumps(record | {
        "generated": str(target), "generated_sha256": hashlib.sha256(text.encode()).hexdigest()},
        indent=2, sort_keys=True) + "\n")
    return target


def install() -> dict:
    out.BASELINE_REFS.setdefault(ALPHA83_REF, {
        "commit": ALPHA83_COMMIT, "baseline": (ALPHA83_REF, 83), "candidate": ("v0.1.0-alpha.84", 84),
        "unpatched": True, "baseline_policy": dict(POLICY_83)})
    out.BASELINE_REFS.setdefault(FRESH_REF, {
        "commit": SOURCE_COMMIT, "baseline": (FRESH_REF, 84), "candidate": ("v0.1.0-alpha.85", 85),
        "unpatched": True, "baseline_policy": dict(POLICY_84)})
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    text = origin_text(original)
    target = _write("worker_fixture_origin.py", text, {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_policies": [POLICY_83, POLICY_84, POLICY_85]})
    spec = importlib.util.spec_from_file_location(fx12.ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[fx12.ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[fx12.ORIGIN_NAME] = module   # replaces set12's copy (a superset of its transitions)
    return {"origin": str(target)}


def configure(document: dict) -> dict:
    """configure_labels for a document; alpha.83 baseline: ledger 44 after the update and after a return, alpha.83's
    HSTS header before the update and after a return."""
    labels = out.configure_labels(document)
    if labels.get("ref") == ALPHA83_REF:
        out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN = 44, 44
        import set6_trial as s6  # noqa: PLC0415  (loaded by every set15 driver already)
        s6.HSTS_ALPHA81 = HSTS_ALPHA83
    return labels


def bound_worker_copy() -> Path:
    source = HERE / "guest_bound_worker.py"
    original = source.read_text()
    if original.count(fx12.PAIRS_LINE) != 1:
        raise ValueError("guest_bound_worker.py differs from the reviewed text; the set15 copy is not made")
    text = original.replace(fx12.PAIRS_LINE, PAIRS_NEW)
    return _write("guest_bound_worker.py", text, {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_pair": ["v0.1.0-alpha.83", "v0.1.0-alpha.84"]})


_provenance_for = out.provenance_for


def provenance_for(variant: str) -> dict:
    value = _provenance_for(variant)
    if out.LABEL_REF == FRESH_REF:
        value = dict(value, baseline=(
            f"set15 fresh cells: the unpublished source commit {SOURCE_COMMIT} itself (its own release policy "
            "v0.1.0-alpha.84 / 84), built with the acceptance_license build tag; installed by the real installer; "
            "fixture trust root enrolled; not a published tag and not the signed release archive"),
            candidate=("unpublished-local-fixture-commit-over-the-source-labelled-v0.1.0-alpha.85; served by the "
                       "guest-loopback origin; never installed in a fresh cell; not a release"))
    return value


out.provenance_for = provenance_for
install()


if __name__ == "__main__":
    raise SystemExit(out.main())
