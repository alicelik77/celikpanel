#!/bin/bash
t=0; while pgrep -f "^bash /mnt/c/.*/set11/tools/stage-common.sh" >/dev/null && [ $t -lt 290 ]; do sleep 5; t=$((t+5)); done; echo "waited=$t still=$(pgrep -fc "^bash /mnt/c/.*/set11/tools/stage-common.sh")"; pgrep -af "git -c" | cut -c1-150
