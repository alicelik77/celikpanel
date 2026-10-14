#!/usr/bin/env python3
"""set16: owner_update_trial with the release profiles of the eighteenth native record (the pre-publication
measurement of the v0.1.0-alpha.85 candidate, commit 0bea00fa0 on fix/alpha85-setup-finish-wait).

Nothing of owner_update_trial.py, worker_fixture_origin.py, guest_bound_worker.py, set12_fixture.py or
set15_fixture.py is changed on disk. Importing this module (or running it as the build's fixture driver) first imports
set15_fixture (which imports set12_fixture), then, in this process only:

* ``BASELINE_REFS["v0.1.0-alpha.84"]`` is REPLACED (set15 used that name as a label for its unpublished source
  1636b73f0): the update cells' baseline is the alpha.84 tag's own commit (f0157e6c; it carries the D-027
  acceptance-licence seam and its guard, so no file is patched); the candidate is labelled ``v0.1.0-alpha.85 / 85``
  (previous = the tag commit) by a fixture commit over the source that changes only
  ``deploy/release-sequence-policy`` (the source's own policy still reads alpha.84).
* ``BASELINE_REFS["v0.1.0-alpha.85"]``: a label profile for the fresh-install cell. Its "baseline" (the build that is
  installed) is that same alpha.85-labelled fixture commit (the update's good candidate, one archive); its commit is
  only known after the build, so ``configure`` takes it from the artifacts document, whose build script proved that
  the commit differs from the source in the release policy only. The origin's candidate (never installed in a fresh
  cell) is a fixture commit labelled ``v0.1.0-alpha.86 / 86``.
* the fixture origin's closed set of target transitions gains 86 (previous 85) besides set15's 83, 84 and 85: a copy
  of worker_fixture_origin.py is written under ``$SET12_RUN_ROOT/generated-set16/`` and replaces set15's copy.
* ``bound_worker_copy()``: guest_bound_worker.py with the (alpha.83, alpha.84) and (alpha.84, alpha.85) pairs added.
* for the alpha.84 baseline, ``LEDGER_AFTER_UPDATE = LEDGER_AFTER_RETURN = 44`` (no migration between the tag and
  the source) and set6's expected HSTS header before the update and after a return is alpha.84's own
  (``cmd/panel/security.go:46`` of the tag: ``max-age=31536000``).
* ``provenance_for`` wraps the ORIGINAL owner_update_trial function again (set15's wrapper would call the alpha.84
  update baseline "the unpublished source commit 1636b73f0").

  set16_fixture.py fixture-source ...   (build-set16-artifacts.sh; the same arguments as owner_update_trial.py)
  set16_fixture.py prove --artifacts A.json

set16: alpha.84 tabanını, alpha.85 etiketli adayı ve taze kurulum için alpha.85 etiket profilini yalnız bu süreçte
tanıtan ince katman; hiçbir dosyayı diskte değiştirmez.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set15_fixture as fx15  # noqa: E402  (registers alpha.82/83 and loads set15's origin copy first)

fx12, out = fx15.fx12, fx15.out
ALPHA84_COMMIT = "f0157e6c8f1614cff176821172f8a9067493e625"
ALPHA84_REF = "v0.1.0-alpha.84"
SOURCE_COMMIT = "0bea00fa041e62610d8d1f433b582b8dca8c9db5"
FRESH_REF = "v0.1.0-alpha.85"
POLICY_84 = dict(fx15.POLICY_84)
POLICY_85 = dict(fx15.POLICY_85)
POLICY_86 = {"version": "v0.1.0-alpha.86", "current": 86, "previous": 85, "previous_version": "v0.1.0-alpha.85"}
HSTS_ALPHA84 = "max-age=31536000"   # cmd/panel/security.go:46 at v0.1.0-alpha.84 (f0157e6c)
PUBLISHED_ALPHA84_SHA256 = "6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301"
GENERATED = fx12.RUN_ROOT / "generated-set16"
HEX40 = re.compile(r"[0-9a-f]{40}")
PAIRS_NEW = ("    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'),"
             " ('v0.1.0-alpha.83', 'v0.1.0-alpha.84'), ('v0.1.0-alpha.84', 'v0.1.0-alpha.85'))  # set16 (generated copy)\n")
UPDATE_PROFILE = {"commit": ALPHA84_COMMIT, "baseline": (ALPHA84_REF, 84), "candidate": ("v0.1.0-alpha.85", 85),
                  "unpatched": True, "baseline_policy": dict(POLICY_84)}


def origin_text(original: str) -> str:
    """The origin's text with HERE pinned to this directory and the 83, 84, 85 and 86 transitions appended."""
    if original.count(fx12.HERE_LINE) != 1 or original.count(fx12.POLICIES_ANCHOR) != 1:
        raise ValueError("worker_fixture_origin.py differs from the reviewed text; the set16 copy is not made")
    text = original.replace(fx12.HERE_LINE, f"HERE = Path({str(HERE)!r})\n")
    added = ("\n# set16 (generated, not a repository file): set15's 83, 84 and 85 transitions, and the fresh cell's "
             "origin\n# candidate labelled v0.1.0-alpha.86 (previous 85).\n"
             f"RELEASE_POLICIES = RELEASE_POLICIES + ({fx15.POLICY_83!r}, {POLICY_84!r}, {POLICY_85!r}, {POLICY_86!r})\n")
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
    out.BASELINE_REFS[ALPHA84_REF] = dict(UPDATE_PROFILE)   # replaces set15's label profile of the same name
    out.BASELINE_REFS.setdefault(FRESH_REF, {
        "commit": None, "baseline": (FRESH_REF, 85), "candidate": ("v0.1.0-alpha.86", 86),
        "unpatched": True, "baseline_policy": dict(POLICY_85)})
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    target = _write("worker_fixture_origin.py", origin_text(original), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_policies": [fx15.POLICY_83, POLICY_84, POLICY_85, POLICY_86]})
    spec = importlib.util.spec_from_file_location(fx12.ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[fx12.ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[fx12.ORIGIN_NAME] = module   # replaces set15's copy (a superset of its transitions)
    return {"origin": str(target)}


def adopt_fresh_commit(document: dict) -> None:
    """The alpha.85 label profile's commit is the document's installed commit (known only after the build)."""
    ref = (document or {}).get("baseline_ref") or {}
    if isinstance(ref, dict) and ref.get("ref") == FRESH_REF:
        commit = ref.get("tag_commit")
        if not HEX40.fullmatch(str(commit or "")) or (document.get("baseline") or {}).get("commit") != commit:
            raise ValueError("the alpha.85 label profile needs the installed commit named in baseline_ref and baseline")
        out.BASELINE_REFS[FRESH_REF]["commit"] = commit


def main(argv: list[str] | None = None) -> int:
    """owner_update_trial's command line; ``prove --artifacts`` of a fresh document first adopts its installed commit."""
    args = list(sys.argv[1:] if argv is None else argv)
    if "--artifacts" in args and args.index("--artifacts") + 1 < len(args):
        try:
            adopt_fresh_commit(json.loads(Path(args[args.index("--artifacts") + 1]).read_text()))
        except (OSError, ValueError):
            pass   # owner_update_trial reports an unreadable or inconsistent document itself
    return out.main(argv)


def configure(document: dict) -> dict:
    """configure_labels for a document. alpha.84 baseline: ledger 44 after the update and after a return, alpha.84's
    HSTS header. alpha.85 label (fresh): the profile's commit is the document's own installed commit."""
    adopt_fresh_commit(document)
    labels = out.configure_labels(document)
    if labels.get("ref") == ALPHA84_REF:
        out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN = 44, 44
        import set6_trial as s6  # noqa: PLC0415  (loaded by every set16 driver already)
        s6.HSTS_ALPHA81 = HSTS_ALPHA84
    return labels


def bound_worker_copy() -> Path:
    source = HERE / "guest_bound_worker.py"
    original = source.read_text()
    if original.count(fx12.PAIRS_LINE) != 1:
        raise ValueError("guest_bound_worker.py differs from the reviewed text; the set16 copy is not made")
    return _write("guest_bound_worker.py", original.replace(fx12.PAIRS_LINE, PAIRS_NEW), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_pairs": [["v0.1.0-alpha.83", "v0.1.0-alpha.84"], ["v0.1.0-alpha.84", "v0.1.0-alpha.85"]]})


_provenance_original = fx15._provenance_for   # owner_update_trial's own function, before set15 wrapped it


def provenance_for(variant: str) -> dict:
    value = _provenance_original(variant)
    if out.LABEL_REF == FRESH_REF:
        value = dict(value, baseline=(
            f"set16 fresh cell: a disposable fixture commit over the unpublished source {SOURCE_COMMIT} that changes only "
            "deploy/release-sequence-policy (v0.1.0-alpha.85 / 85, previous the alpha.84 tag commit); the update cells' "
            "good candidate, one archive; built with the acceptance_license build tag; installed by the real installer; "
            "fixture trust root enrolled; not a published tag and not the signed release archive"),
            candidate=("unpublished-local-fixture-commit-over-the-installed-commit-labelled-v0.1.0-alpha.86; served by "
                       "the guest-loopback origin; never installed in a fresh cell; not a release"))
    elif out.LABEL_REF == ALPHA84_REF:
        value = dict(value, candidate=(
            f"unpublished-local-fixture-commit-over-the-source-{SOURCE_COMMIT[:12]}-labelled-v0.1.0-alpha.85 (release policy "
            "only); acceptance-license build; not a release"))
    return value


out.provenance_for = provenance_for
install()


if __name__ == "__main__":
    raise SystemExit(main())
