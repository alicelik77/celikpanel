#!/bin/bash
# set13: stage what is common to the run (the build, its proof, the dry runs, the run copies and their offline suites,
# host records, tools) and the browser windows' output.
set -uo pipefail
R=/var/tmp/cp-set13-run; L=$R/logs
NAME=$(cat $R/evidence-name.txt)
E="<repo>/deploy/e2e/release-recovery/evidence/$NAME"
J=<scratchpad>/set13/tools
B=<scratchpad>/set13/browser
REPO='<repo>'
G() { git -c safe.directory='*' "$@"; }
SRC=$(G -C "$REPO" rev-parse '816b8dd60^{commit}')
mkdir -p "$E/build/fresh" "$E/harness-run-copy" "$E/host" "$E/tools"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
ART=$(cat $R/artifacts-fresh.path); BD=$(dirname "$ART")
cp "$ART" "$E/build/fresh/"
for r in baseline good defective startcheck realstart; do d=$(field "$ART" $r dist_json); [ -n "$d" ] && [ -f "$d" ] && cp "$d" "$E/build/fresh/dist-$r.json"; done
G -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/fresh/fixture-commits.txt"
for r in baseline good defective; do
  c=$(field "$ART" $r commit); [ -n "$c" ] || continue
  { echo "$r $c tree $(G -C $BD/repo rev-parse "$c^{tree}")"; echo "### files that differ between the source 816b8dd60 and $r"; G -C $BD/repo diff --name-status $SRC $c | head -n 60; } >> "$E/build/fresh/trees.txt"
done
cp $R/build/fresh-prove.json "$E/build/fresh-prove.json"; cp $R/build/fresh-prove.stderr.txt "$E/build/fresh-prove.stderr.txt"
tail -n 40 $L/build-fresh.err > "$E/build/build-fresh.err.tail.txt"; cp $L/build-fresh.out "$E/build/build-fresh.out.txt"
echo "source 816b8dd60 = $SRC tree $(G -C "$REPO" rev-parse "$SRC^{tree}")" > "$E/build/source.txt"
cp $R/host/source-check.txt "$E/build/source-check.txt"
cp $R/build/dry-*.json $R/build/dry-*.err "$E/build/" 2>/dev/null
for y in out start end rc; do cp $L/build.$y "$E/build/job-build.$y.txt" 2>/dev/null; done
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
for o in a b c d e; do
  [ -d $R/overlay-$o ] || continue
  mkdir -p "$E/harness-run-copy/$o"
  for f in files.sha256 harness.diff differs-from-archive.txt runcopy-against-commit.txt; do cp $R/overlay-$o/$f "$E/harness-run-copy/$o/" 2>/dev/null; done
  [ -f $L/suite-$o.txt ] && cp $L/suite-$o.txt "$E/harness-run-copy/$o/suite.txt" && cp $L/suite-$o-notok.txt "$E/harness-run-copy/$o/suite-notok.txt"
done
cp $R/progress.txt "$E/host/progress.txt"; cp $R/c-drive-cells.txt "$E/host/c-drive-cells.txt"; cp $R/neighbour-gate.txt "$E/host/neighbour-gate.txt"
cp $R/host/hostcheck-before.txt "$E/host/hostcheck-before.txt"
cp $J/c-drive-watch.txt "$E/host/c-drive-watch.txt"; cp $J/keepawake.log "$E/host/keepawake.log" 2>/dev/null; cp $J/sleep-events.txt "$E/host/sleep-events.txt" 2>/dev/null
cp $J/../set12-generated-restore.txt "$E/host/set12-generated-restore.txt" 2>/dev/null
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; } > "$E/host/hostcheck.txt"
G -C "$REPO" status --short > "$E/host/working-tree-status.txt"
G -C "$REPO" rev-parse HEAD > "$E/host/head-at-staging.txt"
for f in $J/*; do case $(basename "$f") in c-drive*|keepawake.log|sleep-events.txt|cwatch.stop|keepawake.stop) ;; *) [ -f "$f" ] && cp "$f" "$E/tools/";; esac; done
cp $J/../mkbrowser.py $J/../browser-body.mjs "$E/tools/" 2>/dev/null
cp $B/runner.log "$E/host/browser-runner.log"
for h in h13d13:debian13/fresh-a h13d13b:debian13/fresh-b; do
  n=${h%%:*}; d=${h#*:}
  [ -d $B/$n-ui ] || continue
  mkdir -p "$E/$d/browser/handover"
  cp -r $B/$n-ui/. "$E/$d/browser/"
  cp $R/hand/$n-ui/*.json "$E/$d/browser/handover/" 2>/dev/null
done
echo "staged common at $(date -u +%FT%TZ)"
