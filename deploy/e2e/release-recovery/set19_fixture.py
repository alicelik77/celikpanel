#!/usr/bin/env python3
"""set19: owner_update_trial with the release profile of the Certbot renewal measurement (2026-10-16, the Frankfurt
update of alpha.89 -> alpha.90 that died at "Certbot renewal scheduler state could not be restored").

Nothing of owner_update_trial.py, worker_fixture_origin.py, guest_bound_worker.py or the earlier fixture modules is
changed on disk. Importing this module (or running it as the build's fixture driver) first imports set18b_fixture
(which registers every earlier profile), then, in this process only:

* ``BASELINE_REFS["v0.1.0-alpha.90"]``: the update cells' baseline is the v0.1.0-alpha.90 tag's own commit
  (1743ac69; it carries the D-027 acceptance-licence seam and its guard, so no file is patched), rebuilt locally as a
  test-licence build (not the published archive). The candidate is labelled ``v0.1.0-alpha.92 / 92``: a policy-only
  fixture commit over the source (the source's own policy is 92 after 91; the fixture names the alpha.90 tag commit
  as its predecessor, because the fixture origin serves exactly one transition from the installed baseline).
* the fixture origin's closed set of target transitions gains 92 (previous 90): a copy of
  worker_fixture_origin.py is written under ``$SET12_RUN_ROOT/generated-set19/`` and replaces set18's copy.
* ``bound_worker_copy()``: guest_bound_worker.py with the (alpha.90, alpha.92) pair added.
* ``LEDGER_AFTER_UPDATE = LEDGER_AFTER_RETURN = 44`` (no migration between the alpha.90 tag and the source) and
  set6's expected HSTS header ``max-age=31536000`` (``cmd/panel/security.go:46`` at the tag and at the source).

  set19_fixture.py fixture-source ...   (build-set19-artifacts.sh; the same arguments as owner_update_trial.py)
  set19_fixture.py prove --artifacts A.json

set19: alpha.90 etiketi tabanini ve alpha.92 etiketli adayi yalniz bu surecte tanitan ince katman; diskte hicbir
dosyayi degistirmez.
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
import set18b_fixture as fx18b  # noqa: E402  (registers every earlier profile and loads set18's origin copy first)

fx18 = fx18b.fx18
fx17, fx16, fx15, fx12, out = fx18.fx17, fx18.fx16, fx18.fx15, fx18.fx12, fx18.out
TAG90_COMMIT = "1743ac69edb0e5723dc60d992b53094dcc4b1431"   # v0.1.0-alpha.90 (the Frankfurt update's target)
TAG91_COMMIT = "356109b532b9d0d0df5dd31fc19e9932918a9648"   # v0.1.0-alpha.91 (tagged, not on the portal)
REF90 = "v0.1.0-alpha.90"
SOURCE_COMMIT = None   # set by the trial from the artifacts document (the branch head that was built)
POLICY_90 = {"version": "v0.1.0-alpha.90", "current": 90, "previous": 89, "previous_version": "v0.1.0-alpha.89"}
POLICY_92 = {"version": "v0.1.0-alpha.92", "current": 92, "previous": 90, "previous_version": "v0.1.0-alpha.90"}
UPDATE_PROFILE = {"commit": TAG90_COMMIT, "baseline": (REF90, 90), "candidate": ("v0.1.0-alpha.92", 92),
                  "unpatched": True, "baseline_policy": dict(POLICY_90)}
GENERATED = fx12.RUN_ROOT / "generated-set19"
PAIRS_NEW = ("    pairs = (('v0.1.0-alpha.81', 'v0.1.0-alpha.82'), ('v0.1.0-alpha.80', 'v0.1.0-alpha.81'),"
             " ('v0.1.0-alpha.85', 'v0.1.0-alpha.86'), ('v0.1.0-alpha.86', 'v0.1.0-alpha.87'),"
             " ('v0.1.0-alpha.90', 'v0.1.0-alpha.92'))  # set19 (generated copy)\n")


def origin_text(original: str) -> str:
    """set18's origin text plus the 90 -> 92 transition (exact edit before the same anchor)."""
    text = fx18.origin_text(original)
    if text.count(fx12.POLICIES_ANCHOR) != 1:
        raise ValueError("set18's origin copy differs from the reviewed text; the set19 copy is not made")
    added = ("\n# set19 (generated, not a repository file): the update from the alpha.90 tag to a candidate labelled "
             f"v0.1.0-alpha.92 (previous 90).\nRELEASE_POLICIES = RELEASE_POLICIES + ({POLICY_90!r}, {POLICY_92!r})\n")
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
    out.BASELINE_REFS[REF90] = dict(UPDATE_PROFILE)
    source = HERE / "worker_fixture_origin.py"
    original = source.read_text()
    target = _write("worker_fixture_origin.py", origin_text(original), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_policies": [POLICY_90, POLICY_92]})
    spec = importlib.util.spec_from_file_location(fx12.ORIGIN_NAME, target)
    module = importlib.util.module_from_spec(spec)
    sys.modules[fx12.ORIGIN_NAME] = module
    spec.loader.exec_module(module)
    out._MODULES[fx12.ORIGIN_NAME] = module   # replaces set18's copy (a superset of its transitions)
    return {"origin": str(target)}


def bound_worker_copy() -> Path:
    source = HERE / "guest_bound_worker.py"
    original = source.read_text()
    if original.count(fx12.PAIRS_LINE) != 1:
        raise ValueError("guest_bound_worker.py differs from the reviewed text; the set19 copy is not made")
    return _write("guest_bound_worker.py", original.replace(fx12.PAIRS_LINE, PAIRS_NEW), {
        "original": str(source), "original_sha256": hashlib.sha256(original.encode()).hexdigest(),
        "added_pairs": [["v0.1.0-alpha.90", "v0.1.0-alpha.92"]]})


def main(argv: list[str] | None = None) -> int:
    return out.main(argv)


def configure(document: dict) -> dict:
    """configure_labels for a set19 document: ledger 44 after the update and after a return, and the alpha.90 HSTS
    header before the update and after a return."""
    labels = out.configure_labels(document)
    if labels.get("ref") == REF90:
        out.LEDGER_AFTER_UPDATE, out.LEDGER_AFTER_RETURN = 44, 44
        import set6_trial as s6  # noqa: PLC0415  (loaded by every set19 driver already)
        s6.HSTS_ALPHA81 = fx17.HSTS_ALPHA85   # the same header at the alpha.90 tag and at the source
    return labels


_provenance_set18 = out.provenance_for


def provenance_for(variant: str) -> dict:
    value = _provenance_set18(variant)
    if out.LABEL_REF == REF90:
        value = dict(value, baseline=(
            f"the v0.1.0-alpha.90 tag's own commit {TAG90_COMMIT} unchanged (it carries the acceptance-licence seam), "
            "rebuilt locally as a test-licence build; installed by the real installer; not the published archive"),
            candidate=(f"an unpublished fixture commit over the source {SOURCE_COMMIT or '(the branch head)'} that "
                       "changes only the release policy to v0.1.0-alpha.92 / 92 with the alpha.90 tag commit as its "
                       "predecessor; acceptance-license build; served by the guest-loopback origin; not a release"))
    return value


out.provenance_for = provenance_for
install()


if __name__ == "__main__":
    raise SystemExit(main())
