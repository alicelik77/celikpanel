#!/bin/bash
# set16: the real headless Chrome on the Windows host against the lab's Panel (through the driver's loopback forward).
# usage: browser.sh PREFIX   (Git Bash on the host; bounded by `timeout`; output under the scratchpad set16/browser)
set -u
S=<redacted>
prefix=$1
out=$S/set16/browser/$prefix-ui
log=$S/set16/browser/$prefix.log
[ -e "$out" ] && { echo "refusing: $out exists"; exit 2; }
mkdir -p "$out"
cd "/c/CELIKBROS PROJECTS/celikpanel" || exit 2
export BROWSER_INSPECT_MODULES="$(cygpath -w "$S/set7/browser-modules")"
outw=$(cygpath -w "$out")
hand='\\wsl.localhost\archlinux\var\tmp\cp-set16\hand\'"$prefix"'-ui'
echo "hand=$hand start=$(date -u +%FT%TZ)" > "$log"
MSYS_NO_PATHCONV=1 timeout 5400 node web/tools/browser-inspect/cold-load-set16.mjs "$hand" "$outw" ui,done >> "$log" 2>&1
echo "rc=$? end=$(date -u +%FT%TZ)" >> "$log"
