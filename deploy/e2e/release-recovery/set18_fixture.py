#!/usr/bin/env python3
"""set18: owner_update_trial with the release profile of the twenty-first native record (the alpha.87 candidate,
HEAD 5d54aba1e of feat/alpha87-second-server, installed fresh and set up to ready on a lab with its own DNS and ACME).

Nothing of owner_update_trial.py, worker_fixture_origin.py or the set12/set15/set16/set17 fixture modules is changed
on disk. Importing this module (or running it as the build's fixture driver) first imports set17_fixture (which
registers every earlier profile), then, in this process only:

* ``BASELINE_REFS["v0.1.0-alpha.87"]``: a label profile for the fresh cell. Its "baseline" (the build that is
  installed) is a fixture commit over the source that changes ONLY the release policy to ``v0.1.0-alpha.87 / 87``
  (previous 86 = the v0.1.0-alpha.86 tag commit 8d260f31); its commit is only known after the build, so
  ``configure`` takes it from the artifacts document. The origin's candidate is a second policy-only fixture labelled
  ``v0.1.0-alpha.88 / 88`` (previous 87); it is never installed (set18 starts no update), but the artifacts document
  and the origin step need it.
* the fixture origin's closed set of target transitions gains 88 (previous 87) besides set17's: a copy of
  worker_fixture_origin.py is written under ``$SET12_RUN_ROOT/generated-set18/`` and replaces set17's copy.
* ``provenance_for`` names the set18 baseline.

  set18_fixture.py fixture-source ...   (build-set18-artifacts.sh; the same arguments as owner_update_trial.py)
  set18_fixture.py prove --artifacts A.json

set18: alpha.87 etiketli adayi (kaynak + yalniz surum politikasi) ve kurulmayan alpha.88 etiketini yalniz bu surecte
tanitan ince katman; hicbir dosyayi diskte degistirmez.
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
import set17_fixture as fx17  # noqa: E402  (registers every earlier profile and loads set17's origin copy first)

fx16, fx15, fx12, out = fx17.fx16, fx17.fx15, fx17.fx12, fx17.out
SOURCE_COMMIT = "5d54aba1e9c93357fa0c19d3b0487e27384acd01"   # set18 (2026-10-15): HEAD of feat/alpha87-second-server
TAG86_COMMIT = "8d260f317e58bed7fcd84c6cbb01823c4fe3f166"    # v0.1.0-alpha.86
THIRD_REF = "v0.1.0-alpha.87"
POLICY_87 = dict(fx17.POLICY_87)
POLICY_88 = {"version": "v0.1.0-alpha.88", "current": 88, "previous": 87, "previous_version": "v0.1.0-alpha.87"}
THIRD_PROFILE = {"commit": None, "baseline": (THIRD_REF, 87), "candidate": ("v0.1.0-alpha.88", 88),
                 "unpatched": True, "baseline_policy": dict(POLICY_87)}
GENERATED = fx12.RUN_ROOT / "generated-set18"
HEX40 = re.compile(r"[0-9a-f]{40}")


def origin_text(original: str) -> str:
    """set17's origin text plus the 88 transition (exact edit before the same anchor)."""
    text = fx17.origin_text(original)
    if text.count(fx12.POLICIES_ANCHOR) != 1:
        raise ValueError("set17's origin copy differs from the reviewed text; the set18 copy is not made")
    added = ("\n# set18 (generated, not a repository file): the never-installed origin candidate labelled "
             f"v0.1.0-alpha.88 (previous 87).\nRELEASE_POLICIES = RELEASE_POLICIES + ({POLICY_88!r},)\n")
    return text.replace(fx12.POLICIES_ANCHOR, "\n" + added + fx12.POLICIES_ANCHOR)


def install() -> dict:
    out.BASELINE_REFS[THIRD_REF] = dict(THIRD_PROFILE)
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    text = origin_text(original)
    GENERATED.mkdir(parents=True, exist_ok=True)
    target = GENERATED / "worker_fixture_origin.py"
    if not target.exists() or target.read_text() != text:
        target.write_text(text)
    (GENERATED / "worker_fixture_origin.json").write_text(json.dumps({
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "generated": str(target), "generated_sha256": hashlib.sha256(text.encode()).hexdigest(),
        "added_policies": [fx15.POLICY_83, fx16.POLICY_84, fx17.POLICY_85, fx17.POLICY_86, POLICY_87, POLICY_88]},
        indent=2, sort_keys=True) + "\n")
    spec = importlib.util.spec_from_file_location(fx12.ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[fx12.ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[fx12.ORIGIN_NAME] = module   # replaces set17's copy (a superset of its transitions)
    return {"origin": str(target)}


def adopt_third_commit(document: dict) -> None:
    """The alpha.87 label profile's commit is the document's installed commit (known only after the build)."""
    ref = (document or {}).get("baseline_ref") or {}
    if isinstance(ref, dict) and ref.get("ref") == THIRD_REF:
        commit = ref.get("tag_commit")
        if not HEX40.fullmatch(str(commit or "")) or (document.get("baseline") or {}).get("commit") != commit:
            raise ValueError("the alpha.87 label profile needs the installed commit named in baseline_ref and baseline")
        out.BASELINE_REFS[THIRD_REF]["commit"] = commit


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if "--artifacts" in args and args.index("--artifacts") + 1 < len(args):
        try:
            adopt_third_commit(json.loads(Path(args[args.index("--artifacts") + 1]).read_text()))
        except (OSError, ValueError):
            pass   # owner_update_trial reports an unreadable or inconsistent document itself
    return out.main(argv)


def configure(document: dict) -> dict:
    """configure_labels for a set18 document (ledger 44 as set17: no migration since the alpha.85 tag)."""
    adopt_third_commit(document)
    labels = out.configure_labels(document)
    if labels.get("ref") == THIRD_REF:
        out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN = 44, 44
    return labels


_provenance_set17 = out.provenance_for


def provenance_for(variant: str) -> dict:
    value = _provenance_set17(variant)
    if out.LABEL_REF == THIRD_REF:
        value = dict(value, baseline=(
            f"set18 fresh cell: an unpublished fixture commit over the source {SOURCE_COMMIT} that changes only the "
            f"release policy to v0.1.0-alpha.87 / 87 (previous 86 = the v0.1.0-alpha.86 tag commit {TAG86_COMMIT}); "
            "built with the acceptance_license build tag; installed by the real installer; fixture trust root "
            "enrolled; not a published tag and not a release"),
            candidate=("unpublished-local-fixture-commit-labelled-v0.1.0-alpha.88 (release policy only); served by the "
                       "guest-loopback origin; never installed in set18; not a release"))
    return value


out.provenance_for = provenance_for
install()


if __name__ == "__main__":
    raise SystemExit(main())
