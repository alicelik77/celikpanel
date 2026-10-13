#!/bin/bash
# set15: stage what is common to the run: the build (artifacts document, dist records, fixture commits, proof, dry
# runs), run copy b's overlay and offline suite, host records and the tools of this run.
set -uo pipefail
source <redacted>
E="$REPO/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
mkdir -p "$E/build/fresh" "$E/harness-run-copy/b" "$E/host" "$E/tools"
field() { python3 -I -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],{}).get(sys.argv[3],''))" "$1" "$2" "$3"; }
ART=$(cat $R/artifacts-fresh.path); BD=$(dirname "$ART")
cp "$ART" "$E/build/fresh/"
for r in baseline good defective startcheck realstart; do d=$(field "$ART" $r dist_json); [ -n "$d" ] && [ -f "$d" ] && cp "$d" "$E/build/fresh/dist-$r.json"; done
git -c safe.directory='*' -C $BD/repo for-each-ref --format='%(refname) %(objectname) tree=%(tree) %(subject)' refs/upd1 > "$E/build/fresh/fixture-commits.txt"
SRC=$(G rev-parse "$C^{commit}")
c=$(field "$ART" baseline commit)
{ echo "baseline $c tree $(git -c safe.directory='*' -C $BD/repo rev-parse "$c^{tree}")"; echo "### files that differ between the source $C and the baseline"; git -c safe.directory='*' -C $BD/repo diff --name-status $SRC $c; } > "$E/build/fresh/trees.txt"
cp $R/build/fresh-prove.json $R/build/fresh-prove.stderr.txt $R/build/dry-*.json $R/build/dry-*.err "$E/build/" 2>/dev/null
tail -n 40 $L/build-fresh.err > "$E/build/build-fresh.err.tail.txt"; cp $L/build-fresh.out "$E/build/build-fresh.out.txt"
for y in out start end rc; do cp $L/build.$y "$E/build/job-build.$y.txt" 2>/dev/null; done
cp $R/host/source-check.txt "$E/build/source-check.txt"
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version > "$E/build/go-version.txt"
cp $R/overlay-b/files.sha256 $R/overlay-b/harness.diff "$E/harness-run-copy/b/"; cp $L/suite-b.txt "$E/harness-run-copy/b/suite.txt"; cp $L/mkov-b.txt "$E/harness-run-copy/b/mkov.txt"
cp $R/progress.txt $R/c-drive-cells.txt $R/neighbour-gate.txt "$E/host/" 2>/dev/null
cp $R/host/hostcheck-before.txt "$E/host/"
cp $J/c-drive-watch.txt $J/keepawake.log "$E/host/" 2>/dev/null
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; echo "node (WSL): $(node --version)"; } > "$E/host/hostcheck.txt"
echo "(working-tree status and HEAD at staging are written from the host Git, as in set14b)"
for f in $J/*; do case $(basename "$f") in c-drive*|keepawake.log|cwatch.stop|keepawake.stop|vistest.mjs) ;; *) [ -f "$f" ] && cp "$f" "$E/tools/";; esac; done
cp $J/vistest.mjs "$E/tools/" 2>/dev/null
echo "staged common at $(date -u +%FT%TZ)"
