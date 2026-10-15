#!/bin/bash
# set17b: stage one lab's evidence into the repository's evidence folder: the driver's run directory (its SHA256SUMS
# verified on the staged copy), the lab's host-side records and the browser's output of both windows; then set6's
# digest sweep over the staged copy. Nothing of the lab is removed (the labs are removed after the commit).
# usage: stage-lab.sh JOB LAB NODE GROUP HAND
set -euo pipefail
source <redacted>
job=$1; labname=$2; node=$3; group=$4; hand=$5
lab=/var/tmp/cp-release-drill-$labname
B=$J/browser
E="$REPO/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
case $labname in s17b-*) ;; *) echo "refusing: $labname is not a lab of this run"; exit 2;; esac
[ -e $L/$job.end ] || { echo "refusing: job $job has not ended"; exit 2; }
pgrep -f "cp-release-drill-$labname/" > /dev/null && { echo "refusing: a process of lab $labname is still running"; exit 2; }
REDACT=$R/harness-c/deploy/e2e/release-recovery/set6_redact.py
dst="$E/$group"
[ -e "$dst/driver" ] && { echo "refusing: $dst/driver exists"; exit 2; }
mkdir -p "$dst/driver" "$dst/host" "$dst/browser" "$E/host"
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
for w in page; do
  if [ -d $B/$hand-$w ]; then
    cp -r $B/$hand-$w/. "$dst/browser/"
    cp $B/$hand-$w.log "$dst/browser/node-run-$w.txt"
  fi
  if [ -d $R/hand/$hand-$w ]; then
    mkdir -p "$dst/browser/handover-$w"
    cp $R/hand/$hand-$w/*.json "$dst/browser/handover-$w/" 2>/dev/null || true
  fi
done
[ -n "$(ls -A "$dst/browser")" ] || echo "no browser ran for this lab" > "$dst/browser/no-browser.txt"
python3 -I $REDACT sweep --learn-from "$lab/evidence" "$dst" > $L/stage-sweep-$job.json
python3 -I -c 'import json,sys;r=json.load(open(sys.argv[1]));print("sweep: files changed",len(r["files_changed"]),"inside base64",len(r["files_changed_inside_base64_text"]))' $L/stage-sweep-$job.json
cp $L/stage-sweep-$job.json "$dst/host/digest-sweep-at-staging.json"
python3 -I $REDACT count --learn-from "$lab/evidence" "$dst" > $L/stage-count-$job.json || { echo "A DIGEST OF A TOKEN IS STILL PRESENT after the sweep"; exit 4; }
[ ! -f "$dst/driver/SHA256SUMS" ] || { ( cd "$dst/driver" && sha256sum -c --quiet SHA256SUMS ) && echo "driver SHA256SUMS verified again after the sweep" || echo "driver SHA256SUMS: a file changed in the sweep"; }
du -sh $lab | tee -a $R/lab-sizes.txt
du -sh "$dst"
find "$dst" -type f | wc -l
