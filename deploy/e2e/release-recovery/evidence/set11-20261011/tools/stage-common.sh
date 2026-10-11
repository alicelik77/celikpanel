#!/bin/bash
# set11: stage what is common to the run (both builds, proofs, dry runs, run copies, offline suites, host records, tools).
set -uo pipefail
R=/var/tmp/cp-set11-run; L=$R/logs
NAME=$(cat $R/evidence-name.txt)
E="<repo>/deploy/e2e/release-recovery/evidence/$NAME"
J=<scratchpad>/set11/tools
REPO='<repo>'
G() { git -c safe.directory='*' "$@"; }
SRC=$(G -C "$REPO" rev-parse 'ead69cdcc^{commit}')
mkdir -p "$E/build" "$E/harness-run-copy" "$E/host" "$E/tools"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
for k in fresh a81; do
  ART=$(cat $R/artifacts-$k.path); B=$(dirname "$ART"); mkdir -p "$E/build/$k"
  cp "$ART" "$E/build/$k/"
  for r in baseline good defective startcheck; do d=$(field "$ART" $r dist_json); [ -n "$d" ] && [ -f "$d" ] && cp "$d" "$E/build/$k/dist-$r.json"; done
  G -C $B/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/$k/fixture-commits.txt"
  [ -f $B/baseline-ref-proof.txt ] && cp $B/baseline-ref-proof.txt "$E/build/$k/"
  for r in baseline good defective startcheck; do
    c=$(field "$ART" $r commit); [ -n "$c" ] || continue
    { echo "$r $c tree $(G -C $B/repo rev-parse "$c^{tree}")"; echo "### files that differ between the source ead69cdcc and $r"; G -C $B/repo diff --name-status $SRC $c; } >> "$E/build/$k/trees.txt"
  done
  cp $R/build/$k-prove.json "$E/build/$k-prove.json"; cp $R/build/$k-prove.stderr.txt "$E/build/$k-prove.stderr.txt"
  tail -n 40 $L/build-$k.err > "$E/build/build-$k.err.tail.txt"; cp $L/build-$k.out "$E/build/build-$k.out.txt"
done
echo "source ead69cdcc = $SRC tree $(G -C "$REPO" rev-parse "$SRC^{tree}")" > "$E/build/source.txt"
cp $R/host/source-check.txt "$E/build/source-check.txt"
cp $R/build/dry-*.json "$E/build/" 2>/dev/null
cp $L/build.out "$E/build/build-job.out.txt"; cp $L/build.start "$E/build/build-job.start.txt"; cp $L/build.end "$E/build/build-job.end.txt"
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
for o in a b c d e f g; do
  [ -d $R/overlay-$o ] || continue
  mkdir -p "$E/harness-run-copy/$o"
  for f in files.sha256 harness.diff differs-from-archive.txt runcopy-against-commit.txt; do cp $R/overlay-$o/$f "$E/harness-run-copy/$o/" 2>/dev/null; done
  [ -f $L/suite-$o.txt ] && cp $L/suite-$o.txt "$E/harness-run-copy/$o/suite.txt" && cp $L/suite-$o-notok.txt "$E/harness-run-copy/$o/suite-notok.txt"
done
cp $R/progress.txt "$E/host/progress.txt"; cp $R/c-drive-cells.txt "$E/host/c-drive-cells.txt"; cp $R/neighbour-gate.txt "$E/host/neighbour-gate.txt"
cp $R/host/hostcheck-before.txt "$E/host/hostcheck-before.txt"
cp $J/c-drive-watch.txt "$E/host/c-drive-watch.txt"; cp $J/keepawake.log "$E/host/keepawake.log" 2>/dev/null; cp $J/sleep-events.txt "$E/host/sleep-events.txt" 2>/dev/null
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; } > "$E/host/hostcheck.txt"
G -C "$REPO" status --short > "$E/host/working-tree-status.txt"
G -C "$REPO" rev-parse HEAD > "$E/host/head-at-staging.txt"
for f in $J/*; do case $(basename "$f") in c-drive*|keepawake.log|sleep-events.txt|cwatch.stop|keepawake.stop) ;; *) [ -f "$f" ] && cp "$f" "$E/tools/";; esac; done
echo "staged common at $(date -u +%FT%TZ)"
