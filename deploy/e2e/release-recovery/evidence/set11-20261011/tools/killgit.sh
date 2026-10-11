#!/bin/bash
p=$(pgrep -f "^git -c safe.directory=\* -C <repo> status --short")
echo "git status pid: $p"
[ -n "$p" ] && kill -TERM $p
sleep 20; echo "stage-common still: $(pgrep -fc "^bash /mnt/c/.*/set11/tools/stage-common.sh")"
