#!/bin/bash
# set18: the real headless Chrome on the Windows host against the lab's Panel (through the driver's loopback forward).
# usage: browser18.sh PREFIX WINDOW [WAIT_LIMIT_S] [RUN_LIMIT_S]   (Git Bash on the host)
set -u
S=<redacted>
prefix=$1; window=$2; wait_limit=${3:-5400}; run_limit=${4:-14000}
out=$S/set18/browser/$prefix-$window
log=$S/set18/browser/$prefix-$window.log
[ -e "$out" ] && { echo "refusing: $out exists"; exit 2; }
mkdir -p "$out"
ready=//wsl.localhost/archlinux/var/tmp/cp-set18/hand/$prefix-$window/ready.json
echo "wait for $ready start=$(date -u +%FT%TZ) limit=${wait_limit}s" > "$log"
t=0
until [ -e "$ready" ] || [ $t -ge "$wait_limit" ]; do sleep 3; t=$((t+3)); done
[ -e "$ready" ] || { echo "no handover after ${t}s end=$(date -u +%FT%TZ)" >> "$log"; exit 3; }
echo "handover after ${t}s at $(date -u +%FT%TZ) tool_sha256=$(sha256sum "/c/CELIKBROS PROJECTS/celikpanel/web/tools/browser-inspect/cold-load-set18.mjs" | cut -c1-64)" >> "$log"
cd "/c/CELIKBROS PROJECTS/celikpanel" || exit 2
export BROWSER_INSPECT_MODULES="$(cygpath -w "$S/set7/browser-modules")"
outw=$(cygpath -w "$out")
hand=//wsl.localhost/archlinux/var/tmp/cp-set18/hand/$prefix-$window
MSYS_NO_PATHCONV=1 timeout "$run_limit" node web/tools/browser-inspect/cold-load-set18.mjs "$hand" "$outw" setup,done >> "$log" 2>&1
echo "rc=$? end=$(date -u +%FT%TZ)" >> "$log"
