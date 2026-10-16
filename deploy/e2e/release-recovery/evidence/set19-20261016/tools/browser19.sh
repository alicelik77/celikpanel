#!/bin/bash
# set19: the real headless Chrome on the Windows host against the lab's Panel (through the driver's loopback forward).
# Waits (bounded) for the driver's handover of window WINDOW, then runs the tool once (bounded by `timeout`).
# usage: browser19.sh PREFIX WINDOW CELLS [WAIT_LIMIT_S] [RUN_LIMIT_S]   (Git Bash on the host)
set -u
S=/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad
W=$S/wt92
prefix=$1; window=$2; cells=$3; wait_limit=${4:-5400}; run_limit=${5:-5400}
out=$S/set19/browser/$prefix-$window
log=$S/set19/browser/$prefix-$window.log
[ -e "$out" ] && { echo "refusing: $out exists"; exit 2; }
mkdir -p "$out"
ready=//wsl.localhost/archlinux/var/tmp/cp-set19/hand/$prefix-$window/ready.json
echo "wait for $ready start=$(date -u +%FT%TZ) limit=${wait_limit}s" > "$log"
t=0
until [ -e "$ready" ] || [ $t -ge "$wait_limit" ]; do sleep 3; t=$((t+3)); done
[ -e "$ready" ] || { echo "no handover after ${t}s end=$(date -u +%FT%TZ)" >> "$log"; exit 3; }
echo "handover after ${t}s at $(date -u +%FT%TZ) tool_sha256=$(sha256sum "$W/web/tools/browser-inspect/cold-load-set19.mjs" | cut -c1-64)" >> "$log"
cd "$W" || exit 2
export BROWSER_INSPECT_MODULES="$(cygpath -w "$S/set7/browser-modules")"
outw=$(cygpath -w "$out")
hand=//wsl.localhost/archlinux/var/tmp/cp-set19/hand/$prefix-$window
MSYS_NO_PATHCONV=1 timeout "$run_limit" node web/tools/browser-inspect/cold-load-set19.mjs "$hand" "$outw" "$cells" >> "$log" 2>&1
echo "rc=$? end=$(date -u +%FT%TZ)" >> "$log"
