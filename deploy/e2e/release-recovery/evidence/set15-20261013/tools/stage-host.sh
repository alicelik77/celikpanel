#!/bin/bash
# set15: host records and this run's tools into the evidence folder (reads the run directory and the scratchpad tools).
source <redacted>
E="$REPO/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
mkdir -p "$E/host" "$E/tools" "$E/harness-run-copy"
cp $R/progress.txt $R/c-drive-cells.txt $R/neighbour-gate.txt $R/host/hostcheck-before.txt "$E/host/" 2>/dev/null
cp $J/c-drive-watch.txt $J/keepawake.log $J/sleep-events.txt "$E/host/" 2>/dev/null
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; echo "node (WSL): $(node --version 2>/dev/null)"; } > "$E/host/hostcheck.txt"
for x in c d e; do mkdir -p "$E/harness-run-copy/$x"; cp $R/overlay-$x/files.sha256 $R/overlay-$x/harness.diff "$E/harness-run-copy/$x/"; cp $L/mkov-$x.txt "$E/harness-run-copy/$x/mkov.txt" 2>/dev/null; done
cp $L/suite-d.txt "$E/harness-run-copy/d/suite.txt"; cp $L/suite-e.txt "$E/harness-run-copy/e/suite.txt"
cp $R/build/dry-d-*.json $R/build/dry-e-*.json "$E/build/" 2>/dev/null
for f in $J/*; do case $(basename "$f") in c-drive*|keepawake.log|cwatch.stop|keepawake.stop|sleep-events.txt) ;; *) [ -f "$f" ] && cp "$f" "$E/tools/";; esac; done
echo "staged host at $(date -u +%FT%TZ)"
find "$E" -type f | wc -l
