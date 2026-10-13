#!/bin/bash
# set15: the real headless Chrome on the Windows host against the lab's Panel (through the driver's loopback forward).
# usage: browser.sh PREFIX   (Git Bash on the host; bounded by `timeout`; output under scratchpad set15/browser)
set -u
S=<redacted>
prefix=$1
out=$S/set15/browser/$prefix-ui
log=$S/set15/browser/$prefix.log
[ -e "$out" ] && { echo "refusing: $out exists"; exit 2; }
mkdir -p "$out"
cd "<redacted>" || exit 2
export BROWSER_INSPECT_MODULES="$(cygpath -w "$S/set7/browser-modules")"
outw=$(cygpath -w "$out")
hand='\\wsl.localhost\archlinux\var\tmp\cp-set15-run\hand\'"$prefix"'-ui'
echo "hand=$hand" > "$log"
MSYS_NO_PATHCONV=1 timeout 5400 node web/tools/browser-inspect/cold-load-set15.mjs "$hand" "$outw" ui,done >> "$log" 2>&1
echo "rc=$?" >> "$log"
