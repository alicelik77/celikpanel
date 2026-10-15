#!/usr/bin/env python3
"""set17: owner_update_trial with the release profiles of the nineteenth native record (the pre-publication
measurement of the v0.1.0-alpha.86 candidate, commit 313f3eea2 on fix/alpha86-update-restart-notice).

Nothing of owner_update_trial.py, worker_fixture_origin.py, guest_bound_worker.py or the set12/set15/set16 fixture
modules is changed on disk. Importing this module (or running it as the build's fixture driver) first imports
set16_fixture (which imports set15_fixture and set12_fixture), then, in this process only:

* ``BASELINE_REFS["v0.1.0-alpha.85"]`` is REPLACED (set16 used that name as a label for its fresh cell): the update
  cells' baseline is the alpha.85 tag's own commit (36c33c45; it carries the D-027 acceptance-licence seam and its
  guard, so no file is patched); the candidate is ``v0.1.0-alpha.86 / 86``. The source commit 313f3eea2 already
  carries that release policy itself (``previous_commit`` = the tag commit), so the candidate IS the source commit:
  no fixture commit labels it (the build script refuses a source whose policy is not exactly that one).
* ``BASELINE_REFS["v0.1.0-alpha.86"]``: a label profile for the fresh cell and for the second update. Its
  "baseline" (the build that is installed) is the candidate commit itself; its commit is only known after the
  build, so ``configure`` takes it from the artifacts document, whose build script proved that it is the source
  commit. The origin's candidate is a fixture commit labelled ``v0.1.0-alpha.87 / 87`` (policy only), installed only
  by the second update (cell ``upd86-debian13-good``).
* the fixture origin's closed set of target transitions gains 86 (previous 85) and 87 (previous 86) besides set16's
  83, 84, 85 and 86: a copy of worker_fixture_origin.py is written under ``$SET12_RUN_ROOT/generated-set17/`` and
  replaces set16's copy.
* ``bound_worker_copy()``: guest_bound_worker.py with the (alpha.85, alpha.86) and (alpha.86, alpha.87) pairs added.
* for both profiles ``LEDGER_AFTER_UPDATE = LEDGER_AFTER_RETURN = 44`` (no migration between the alpha.85 tag and
  the source) and set6's expected HSTS header before the update and after a return is ``max-age=31536000``
  (``cmd/panel/security.go:46`` at the tag and at the source).
* ``provenance_for`` wraps the ORIGINAL owner_update_trial function again.

  set17_fixture.py fixture-source ...   (build-set17-artifacts.sh; the same arguments as owner_update_trial.py)
  set17_fixture.py prove --artifacts A.json

set17: alpha.85 etiketi tabanını, alpha.86 adayını (kaynak işlemenin kendisi) ve ikinci güncelleme için alpha.87
etiketini yalnız bu süreçte tanıtan ince katman; hiçbir dosyayı diskte değiştirmez.
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
import set16_fixture as fx16  # noqa: E402  (registers set15/set16 profiles and loads set16's origin copy first)

fx15, fx12, out = fx16.fx15, fx16.fx12, fx16.out
ALPHA85_COMMIT = "36c33c4531ee6c1306d324db7a666403176f3bf7"
ALPHA85_REF = "v0.1.0-alpha.85"
SOURCE_COMMIT = "313f3eea2c9b89251588648b09484cd34b7aecb1"   # set17 (2026-10-15): HEAD of fix/alpha86-update-restart-notice
PRODUCT_COMMIT = "21f07e7a8"   # the last product commit; 313f3eea2 changes no file under cmd/, internal/ or web/
SECOND_REF = "v0.1.0-alpha.86"
POLICY_85 = dict(fx16.POLICY_85)
POLICY_86 = dict(fx16.POLICY_86)
POLICY_87 = {"version": "v0.1.0-alpha.87", "current": 87, "previous": 86, "previous_version": "v0.1.0-alpha.86"}
HSTS_ALPHA85 = "max-age=31536000"   # cmd/panel/security.go:46 at v0.1.0-alpha.85 (36c33c45) and at 313f3eea2
PUBLISHED_ALPHA85_SHA256 = "cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792"
GENERATED = fx12.RUN_ROOT / "generated-set17"
HEX40 = re.compile(r"[0-9a-f]{40}")
PAIRS_NEW = ("    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'),"
             " ('v0.1.0-alpha.85', 'v0.1.0-alpha.86'), ('v0.1.0-alpha.86', 'v0.1.0-alpha.87'))  # set17 (generated copy)\n")
UPDATE_PROFILE = {"commit": ALPHA85_COMMIT, "baseline": (ALPHA85_REF, 85), "candidate": ("v0.1.0-alpha.86", 86),
                  "unpatched": True, "baseline_policy": dict(POLICY_85)}
SECOND_PROFILE = {"commit": None, "baseline": (SECOND_REF, 86), "candidate": ("v0.1.0-alpha.87", 87),
                  "unpatched": True, "baseline_policy": dict(POLICY_86)}


def origin_text(original: str) -> str:
    """The origin's text with HERE pinned to this directory and the 83 to 87 transitions appended (exact edits)."""
    if original.count(fx12.HERE_LINE) != 1 or original.count(fx12.POLICIES_ANCHOR) != 1:
        raise ValueError("worker_fixture_origin.py differs from the reviewed text; the set17 copy is not made")
    text = original.replace(fx12.HERE_LINE, f"HERE = Path({str(HERE)!r})\n")
    added = ("\n# set17 (generated, not a repository file): set16's 83, 84, 85 and 86 transitions, the update from the "
             "alpha.85 tag\n# to the candidate (86, previous 85) and the second update to a label v0.1.0-alpha.87 "
             "(previous 86).\n"
             f"RELEASE_POLICIES = RELEASE_POLICIES + ({fx15.POLICY_83!r}, {fx16.POLICY_84!r}, {POLICY_85!r}, "
             f"{POLICY_86!r}, {POLICY_87!r})\n")
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
    out.BASELINE_REFS[ALPHA85_REF] = dict(UPDATE_PROFILE)    # replaces set16's fresh label profile of the same name
    out.BASELINE_REFS[SECOND_REF] = dict(SECOND_PROFILE)
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    target = _write("worker_fixture_origin.py", origin_text(original), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_policies": [fx15.POLICY_83, fx16.POLICY_84, POLICY_85, POLICY_86, POLICY_87]})
    spec = importlib.util.spec_from_file_location(fx12.ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[fx12.ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[fx12.ORIGIN_NAME] = module   # replaces set16's copy (a superset of its transitions)
    return {"origin": str(target)}


def adopt_second_commit(document: dict) -> None:
    """The alpha.86 label profile's commit is the document's installed commit (known only after the build)."""
    ref = (document or {}).get("baseline_ref") or {}
    if isinstance(ref, dict) and ref.get("ref") == SECOND_REF:
        commit = ref.get("tag_commit")
        if not HEX40.fullmatch(str(commit or "")) or (document.get("baseline") or {}).get("commit") != commit:
            raise ValueError("the alpha.86 label profile needs the installed commit named in baseline_ref and baseline")
        out.BASELINE_REFS[SECOND_REF]["commit"] = commit


def main(argv: list[str] | None = None) -> int:
    """owner_update_trial's command line; ``prove --artifacts`` of a second document first adopts its installed commit."""
    args = list(sys.argv[1:] if argv is None else argv)
    if "--artifacts" in args and args.index("--artifacts") + 1 < len(args):
        try:
            adopt_second_commit(json.loads(Path(args[args.index("--artifacts") + 1]).read_text()))
        except (OSError, ValueError):
            pass   # owner_update_trial reports an unreadable or inconsistent document itself
    return out.main(argv)


def configure(document: dict) -> dict:
    """configure_labels for a document: ledger 44 after the update and after a return, and the alpha.85 HSTS header
    before the update and after a return, for both profiles; the alpha.86 label's commit is the document's own."""
    adopt_second_commit(document)
    labels = out.configure_labels(document)
    if labels.get("ref") in (ALPHA85_REF, SECOND_REF):
        out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN = 44, 44
        import set6_trial as s6  # noqa: PLC0415  (loaded by every set17 driver already)
        s6.HSTS_ALPHA81 = HSTS_ALPHA85
    return labels


def bound_worker_copy() -> Path:
    source = HERE / "guest_bound_worker.py"
    original = source.read_text()
    if original.count(fx12.PAIRS_LINE) != 1:
        raise ValueError("guest_bound_worker.py differs from the reviewed text; the set17 copy is not made")
    return _write("guest_bound_worker.py", original.replace(fx12.PAIRS_LINE, PAIRS_NEW), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_pairs": [["v0.1.0-alpha.85", "v0.1.0-alpha.86"], ["v0.1.0-alpha.86", "v0.1.0-alpha.87"]]})


_provenance_original = fx15._provenance_for   # owner_update_trial's own function, before set15 wrapped it


def provenance_for(variant: str) -> dict:
    value = _provenance_original(variant)
    if out.LABEL_REF == SECOND_REF:
        value = dict(value, baseline=(
            f"set17 fresh cell and second update: the unpublished source commit {SOURCE_COMMIT} itself (its own release "
            "policy v0.1.0-alpha.86 / 86, previous the alpha.85 tag commit); built with the acceptance_license build "
            "tag; installed by the real installer; fixture trust root enrolled; not a published tag and not the signed "
            "release archive"),
            candidate=("unpublished-local-fixture-commit-over-the-source-labelled-v0.1.0-alpha.87 (release policy only); "
                       "served by the guest-loopback origin; installed only by the second update; not a release"))
    elif out.LABEL_REF == ALPHA85_REF:
        value = dict(value, candidate=(
            f"the unpublished source commit {SOURCE_COMMIT[:12]} itself, whose own release policy is v0.1.0-alpha.86 / 86 "
            "after the alpha.85 tag commit (no fixture commit); acceptance-license build; not a release"))
    return value


out.provenance_for = provenance_for
install()


if __name__ == "__main__":
    raise SystemExit(main())
