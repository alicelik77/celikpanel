#!/bin/bash
# set19: stage the run's records into the worktree's evidence folder (run on the QEMU host as root).
# usage: stage.sh LAB_NAME DEST_CELL_NAME   (per lab)  |  stage.sh common
set -uo pipefail
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
E=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/wt92/deploy/e2e/release-recovery/evidence/set19-20261016
mkdir -p $E
if [ "$1" = common ]; then
  mkdir -p $E/build $E/host $E/harness-run-copy/b $E/tools
  A=$(cat $R/artifacts.path); BD=$(dirname "$A")
  cp "$A" "$BD/baseline-ref-proof.txt" $E/build/
  for r in baseline good defective; do dj=$(python3 -I -c "import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]]['dist_json'])" "$A" $r); cp "$dj" $E/build/dist-$r.json; done
  git -c safe.directory='*' -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > $E/build/fixture-commits.txt
  { for r in baseline good defective; do a=$(python3 -I -c "import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]]['archive'])" "$A" $r); echo "== $r $(basename "$a") sha256=$(sha256sum "$a" | cut -c1-64)"; tar -xOzf "$a" --wildcards '*/deploy/release-sequence-policy' 2>/dev/null; done; } > $E/build/archive-pins.txt 2>&1
  cp $R/build/prove.json $R/build/prove.stderr.txt $R/build/dry-*.json $R/build/dry-*.err $E/build/ 2>/dev/null
  tail -n 40 $L/build-set19.err > $E/build/build-set19.err.tail.txt; cp $L/build-set19.out $E/build/build-set19.out.txt
  for y in start end rc; do cp $L/build.$y $E/build/job-build.$y.txt 2>/dev/null; done
  cp $R/host/source-check.txt $R/host/hostcheck.txt $R/host/harness-file-count.txt $E/host/
  cp $R/progress.txt $R/neighbour-gate.txt $E/host/ 2>/dev/null
  cp $R/overlay-b/files.sha256 $R/overlay-b/harness.diff $E/harness-run-copy/b/
  for f in /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/*.sh; do cp "$f" $E/tools/; done
  echo "staged common at $(date -u +%FT%TZ)"
else
  lab=/var/tmp/cp-release-drill-$1; dest=$E/debian13/$2
  [ -e $dest ] && { echo "refusing: $dest exists"; exit 2; }
  mkdir -p $dest
  D=$(find $lab/evidence -maxdepth 4 -type d -name 'upd1-debian13-good-*' | head -1)
  cp -a "$D" $dest/driver
  cp $L/cell-$3.out $dest/cell.out.txt; cp $L/cell-$3.err $dest/cell.err.txt 2>/dev/null; cp $L/cell-$3.rc $dest/cell.rc.txt 2>/dev/null
  du -sh $dest
  echo "staged $1 -> $dest at $(date -u +%FT%TZ)"
fi
