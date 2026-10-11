#!/bin/bash
# set12: stage what is common to the run (the three builds, proofs, dry runs, run copies, offline suites, host records,
# the generated harness copies, tools) and the browser windows' output.
set -uo pipefail
R=/var/tmp/cp-set12-run; L=$R/logs
NAME=$(cat $R/evidence-name.txt)
E="<repo>/deploy/e2e/release-recovery/evidence/$NAME"
J=<scratchpad>/set12/tools
B=<scratchpad>/set12/browser
REPO='<repo>'
G() { git -c safe.directory='*' "$@"; }
SRC=$(G -C "$REPO" rev-parse '8fabc03b3^{commit}')
mkdir -p "$E/build" "$E/harness-run-copy" "$E/host" "$E/tools" "$E/build/generated"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
for k in fresh a81 a82; do
  ART=$(cat $R/artifacts-$k.path); BD=$(dirname "$ART"); mkdir -p "$E/build/$k"
  cp "$ART" "$E/build/$k/"
  for r in baseline good defective startcheck realstart; do d=$(field "$ART" $r dist_json); [ -n "$d" ] && [ -f "$d" ] && cp "$d" "$E/build/$k/dist-$r.json"; done
  G -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/$k/fixture-commits.txt"
  [ -f $BD/baseline-ref-proof.txt ] && cp $BD/baseline-ref-proof.txt "$E/build/$k/"
  for r in baseline good defective; do
    c=$(field "$ART" $r commit); [ -n "$c" ] || continue
    { echo "$r $c tree $(G -C $BD/repo rev-parse "$c^{tree}")"; echo "### files that differ between the source 8fabc03b3 and $r"; G -C $BD/repo diff --name-status $SRC $c | head -n 60; } >> "$E/build/$k/trees.txt"
  done
  cp $R/build/$k-prove.json "$E/build/$k-prove.json"; cp $R/build/$k-prove.stderr.txt "$E/build/$k-prove.stderr.txt"
  tail -n 40 $L/build-$k.err > "$E/build/build-$k.err.tail.txt"; cp $L/build-$k.out "$E/build/build-$k.out.txt"
done
echo "source 8fabc03b3 = $SRC tree $(G -C "$REPO" rev-parse "$SRC^{tree}")" > "$E/build/source.txt"
cp $R/host/source-check.txt "$E/build/source-check.txt"
cp $R/build/dry-*.json "$E/build/" 2>/dev/null
for x in build build82; do for y in out start end rc; do cp $L/$x.$y "$E/build/job-$x.$y.txt" 2>/dev/null; done; done
for f in $R/generated/*; do cp "$f" "$E/build/generated/"; done
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
for o in a b c d e f; do
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
cp $B/runner.log "$E/host/browser-runner.log"
mkdir -p "$E/debian13/fresh-a/browser" "$E/debian13/fresh-b/browser"
cp -r $B/h12d13a-ui/. "$E/debian13/fresh-a/browser/"
cp -r $B/h12d13b-ui/. "$E/debian13/fresh-b/browser/"
for l in s12-d13-a s12-d13-b; do [ -d /var/tmp/cp-set12-run/hand ] && ls /var/tmp/cp-set12-run/hand > /dev/null; done
for h in /var/tmp/cp-set12-run/hand/*; do n=$(basename $h); case $n in h12d13a-*) d=fresh-a;; h12d13b-*) d=fresh-b;; *) continue;; esac; mkdir -p "$E/debian13/$d/browser/handover"; cp $h/*.json "$E/debian13/$d/browser/handover/" 2>/dev/null; done
echo "staged common at $(date -u +%FT%TZ)"
