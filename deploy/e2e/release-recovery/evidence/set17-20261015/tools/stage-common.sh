#!/bin/bash
# set17: stage the run's common records into the repository's evidence folder: the builds (both artifacts
# documents, dist records, fixture commits, trees, proof, pins, dry runs, logs), the run copies' overlays and offline
# suites, host records and this run's tools. The evidence name is in $R/evidence-name.txt.
set -uo pipefail
source <redacted>
NAME=$(cat $R/evidence-name.txt)
E="$REPO/deploy/e2e/release-recovery/evidence/$NAME"
[ -e "$E/build" ] && { echo "refusing: $E/build exists"; exit 2; }
mkdir -p "$E/build" "$E/harness-run-copy" "$E/host" "$E/tools"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
for d in update second; do
  ART=$(cat $R/artifacts-$d.path); BD=$(dirname "$ART"); mkdir -p "$E/build/$d"
  cp "$ART" "$E/build/$d/"; cp "$BD/baseline-ref-proof.txt" "$E/build/$d/"
  for r in baseline good defective; do dj=$(field "$ART" $r dist_json); [ -f "$dj" ] && cp "$dj" "$E/build/$d/dist-$r.json"; done
  git -c safe.directory='*' -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/$d/fixture-commits.txt"
  { for r in baseline good defective; do c=$(field "$ART" $r commit); echo "### $r $c tree $(git -c safe.directory='*' -C $BD/repo rev-parse "$c^{tree}"): files that differ from the source $C"; git -c safe.directory='*' -C $BD/repo diff --name-status $C $c | head -n 60; done; } > "$E/build/$d/trees.txt"
  tail -n 40 $L/build-$d.err > "$E/build/build-$d.err.tail.txt"; cp $L/build-$d.out "$E/build/build-$d.out.txt"
done
cp $R/build/*.json $R/build/*.err $R/build/*.txt "$E/build/" 2>/dev/null
for y in out start end rc; do cp $L/build.$y "$E/build/job-build.$y.txt" 2>/dev/null; done
cp $R/host/source-check.txt "$E/build/source-check.txt"
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
bash $J/distinfo.sh > "$E/build/dist-folders.txt" 2>&1
for x in b c; do mkdir -p "$E/harness-run-copy/$x"; cp $R/overlay-$x/files.sha256 $R/overlay-$x/harness.diff "$E/harness-run-copy/$x/"; cp $L/mkov-$x.txt "$E/harness-run-copy/$x/mkov.txt" 2>/dev/null; cp $L/suite-$x.txt "$E/harness-run-copy/$x/suite.txt" 2>/dev/null; done
cp $R/progress.txt $R/c-drive-cells.txt $R/neighbour-gate.txt $R/host/hostcheck-before.txt $R/lab-sizes.txt "$E/host/" 2>/dev/null
cp $J/c-drive-watch.txt $J/keepawake.log "$E/host/" 2>/dev/null
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; echo "node (WSL): $(node --version 2>/dev/null)"; echo "/dev/kvm: $(ls -l /dev/kvm)"; echo "svm/vmx processors: $(grep -c -E ' (vmx|svm) ' /proc/cpuinfo) of $(nproc)"; } > "$E/host/hostcheck.txt"
for f in $J/*; do case $(basename "$f") in c-drive*|keepawake.log|cwatch.stop|keepawake.stop) ;; *) [ -f "$f" ] && cp "$f" "$E/tools/";; esac; done
echo "staged at $(date -u +%FT%TZ)"; find "$E" -type f | wc -l
