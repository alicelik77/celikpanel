#!/bin/bash
# set11: cell set11-debian13, third lab (run copy e: the probe timing check corrected; browser windows h11d13c).
export PYTHONDONTWRITEBYTECODE=1
export SET11_NEIGHBOUR_GATE=<scratchpad>/set11/tools/neighbour.sh
export SET11_DISK_GATE=<scratchpad>/set11/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET11_HAND=h11d13c
exec bash /var/tmp/cp-set11-run/harness-e/deploy/e2e/release-recovery/run-set11.sh cell set11-debian13 "$(cat /var/tmp/cp-set11-run/artifacts-fresh.path)" s11-d13-c 4871 18871
