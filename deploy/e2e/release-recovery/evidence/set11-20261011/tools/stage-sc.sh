#!/bin/bash
J=<scratchpad>/set11/tools
echo "qemu now: $(pgrep -fc qemu-system)"
bash $J/stage-lab.sh cell-sc s11-sc-a debian13 debian13/update-startcheck 2>&1 | tail -n 8
