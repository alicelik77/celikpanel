#!/bin/bash
# set14c: stage one lab's evidence into the repository's evidence folder: the driver's run directory (its SHA256SUMS
# verified on the staged copy), the lab's host-side records and the browser's output; then set6's digest sweep over
# the staged copy; only then are THIS run's overlay disks and base-image links of that lab removed (host/removals.txt).
# usage: stage-lab.sh JOB LAB NODE GROUP HAND
set -euo pipefail
source <redacted>
job=$1; labname=$2; node=$3; group=$4; hand=$5
lab=/var/tmp/cp-release-drill-$labname
CACHE=/var/tmp/cp-v3n28/images
B=$J/../browser
E="$REPO/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
case $labname in s14c-*) ;; *) echo "refusing: $labname is not a lab of this run"; exit 2;; esac
[ -e $L/$job.end ] || { echo "refusing: job $job has not ended"; exit 2; }
pgrep -f "cp-release-drill-$labname/" > /dev/null && { echo "refusing: a process of lab $labname is still running"; exit 2; }
REDACT=$R/harness-b/deploy/e2e/release-recovery/set6_redact.py
dst="$E/$group"
[ -e "$dst/driver" ] && { echo "refusing: $dst/driver exists"; exit 2; }
mkdir -p "$dst/driver" "$dst/host" "$dst/browser/handover" "$E/host"
src=$(ls -d $lab/evidence/$node/upd1/*/ | tail -1)
cp -r "$src". "$dst/driver/"
if [ -f "$dst/driver/SHA256SUMS" ]; then
  ( cd "$dst/driver" && sha256sum -c --quiet SHA256SUMS ) && echo "driver SHA256SUMS verified on the staged copy ($(wc -l < "$dst/driver/SHA256SUMS") files)"
else
  echo "the driver did not finalize this run (no driver SHA256SUMS)" | tee "$dst/host/driver-not-finalized.txt"
fi
{ echo "lab=$lab"; basename "$src"; } > "$dst/host/lab.txt"
for f in $lab/evidence/$node/*.json; do [ -f "$f" ] && cp "$f" "$dst/host/" || true; done
cp $lab/cells/*/fixture-plan.json "$dst/host/" 2>/dev/null || true
for x in start end rc out err; do cp $L/$job.$x "$dst/host/wrapper.$x.txt"; done
cp $R/job-$job.sh "$dst/host/job.sh"
grep "$labname" $R/c-drive-cells.txt > "$dst/host/c-drive-gate.txt" || true
cp -r $B/$hand-ui/. "$dst/browser/"
cp $R/hand/$hand-ui/*.json "$dst/browser/handover/" 2>/dev/null || true
cp $B/$hand.log "$dst/browser/node-run.txt"
python3 -I $REDACT sweep --learn-from "$lab/evidence" "$dst" > $L/stage-sweep-$job.json
python3 -I -c 'import json,sys;r=json.load(open(sys.argv[1]));print("sweep: files changed",len(r["files_changed"]),"inside base64",len(r["files_changed_inside_base64_text"]))' $L/stage-sweep-$job.json
cp $L/stage-sweep-$job.json "$dst/host/digest-sweep-at-staging.json"
python3 -I $REDACT count --learn-from "$lab/evidence" "$dst" > $L/stage-count-$job.json || { echo "A DIGEST OF A TOKEN IS STILL PRESENT after the sweep; overlays are kept"; exit 4; }
[ ! -f "$dst/driver/SHA256SUMS" ] || { ( cd "$dst/driver" && sha256sum -c --quiet SHA256SUMS ) && echo "driver SHA256SUMS verified again after the sweep" || echo "driver SHA256SUMS: a file changed in the sweep"; }
for d in $lab/cells/*/*/overlay.qcow2; do
  [ -f "$d" ] || continue
  b=$(stat -c %s "$d")
  rm -f -- "${d:?}"
  echo "$(date -u +%FT%TZ) removed $d bytes=$b (overlay disk of this run's lab)" | tee -a "$E/host/removals.txt"
done
for d in $lab/images/*; do
  [ -f "$d" ] || continue
  c="$CACHE/$(basename "$d")"
  if [ -f "$c" ] && [ "$d" -ef "$c" ]; then rm -f -- "${d:?}"; echo "$(date -u +%FT%TZ) removed $d (hard link to the cached base image; the cached file stays)" | tee -a "$E/host/removals.txt"
  else echo "$(date -u +%FT%TZ) KEPT $d (not a link to a cache file)" | tee -a "$E/host/removals.txt"; fi
done
find "$dst" -type f | wc -l
