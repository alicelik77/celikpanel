#!/usr/bin/env python3
"""set18b: set18_fixture with the source of the twenty-second native record (HEAD 6b363ac46 of
feat/alpha87-second-server, the set18 D1/O1/O3 fix). The release profile is set18's unchanged (installed: a
policy-only fixture commit labelled v0.1.0-alpha.87 / 87, previous 86 = the v0.1.0-alpha.86 tag commit 8d260f31;
the origin's never-installed candidate labelled v0.1.0-alpha.88 / 88); only the source named in the provenance text
differs. Nothing of set18_fixture.py or the earlier fixture modules is changed on disk.

  set18b_fixture.py fixture-source ...   (build-set18b-artifacts.sh; the same arguments as owner_update_trial.py)
  set18b_fixture.py prove --artifacts A.json

set18b: set18 surum profili, kaynak olarak 6b363ac46; diskte hicbir dosyayi degistirmez.
"""
from __future__ import annotations

from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set18_fixture as fx18  # noqa: E402  (registers set18's alpha.87 profile and its 88 origin copy)

SOURCE_COMMIT = "6b363ac4680146f856642cffdab0b57662f6159c"   # set18b (2026-10-16): HEAD of feat/alpha87-second-server
fx18.SOURCE_COMMIT = SOURCE_COMMIT   # read at call time by set18's provenance text
out, configure, THIRD_REF = fx18.out, fx18.configure, fx18.THIRD_REF


def main(argv: list[str] | None = None) -> int:
    return fx18.main(argv)


if __name__ == "__main__":
    raise SystemExit(main())
