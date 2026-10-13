#!/bin/bash
# set15: stage the build records of both documents and the run copy's overlay and offline suite (reads the run directory).
set -uo pipefail
source <redacted>
E="$REPO/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
mkdir -p "$E/build" "$E/harness-run-copy/b" "$E/host" "$E/tools"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
SRC=$(G rev-parse "$C^{commit}")
for d in update fresh; do
  ART=$(cat $R/artifacts-$d.path); BD=$(dirname "$ART")
  mkdir -p "$E/build/$d"
  cp "$ART" "$E/build/$d/"
  cp "$BD/baseline-ref-proof.txt" "$E/build/$d/" 2>/dev/null
  for r in baseline good defective; do dj=$(field "$ART" $r dist_json); [ -n "$dj" ] && [ -f "$dj" ] && cp "$dj" "$E/build/$d/dist-$r.json"; done
  git -c safe.directory='*' -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/$d/fixture-commits.txt"
  { for r in baseline good defective; do c=$(field "$ART" $r commit); echo "$r $c tree $(git -c safe.directory='*' -C $BD/repo rev-parse "$c^{tree}")"; echo "### files that differ between the source $SRC and $r"; git -c safe.directory='*' -C $BD/repo diff --name-status $SRC $c; done; } > "$E/build/$d/trees.txt"
  cp $R/build/$d-prove.json $R/build/$d-prove.stderr.txt "$E/build/" 2>/dev/null
  tail -n 60 $L/build-$d.err > "$E/build/build-$d.err.tail.txt"; cp $L/build-$d.out "$E/build/build-$d.out.txt"
done
cp $R/build/dry-*.json $R/build/dry-*.err $R/build/archive-pins.txt "$E/build/" 2>/dev/null
for y in out start end rc; do cp $L/build.$y "$E/build/job-build.$y.txt" 2>/dev/null; done
cp $R/host/source-check.txt "$E/build/source-check.txt"
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
cp $R/generated-set15/* "$E/build/" 2>/dev/null; mkdir -p "$E/build/generated-set15"; mv "$E/build/"worker_fixture_origin.* "$E/build/"guest_bound_worker.* "$E/build/generated-set15/" 2>/dev/null
cp $R/overlay-b/files.sha256 $R/overlay-b/harness.diff "$E/harness-run-copy/b/"; cp $L/suite-b.txt "$E/harness-run-copy/b/suite.txt"; cp $L/mkov-b.txt "$E/harness-run-copy/b/mkov.txt"
echo "staged build at $(date -u +%FT%TZ)"
