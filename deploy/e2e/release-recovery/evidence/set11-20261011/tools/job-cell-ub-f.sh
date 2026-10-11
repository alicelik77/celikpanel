#!/bin/bash
# set11: cell set11-ubuntu (run copy f: the probe timing kept as detail; browser windows h11ub).
export PYTHONDONTWRITEBYTECODE=1
export SET11_NEIGHBOUR_GATE=<scratchpad>/set11/tools/neighbour.sh
export SET11_DISK_GATE=<scratchpad>/set11/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET11_HAND=h11ub
exec bash /var/tmp/cp-set11-run/harness-f/deploy/e2e/release-recovery/run-set11.sh cell set11-ubuntu "$(cat /var/tmp/cp-set11-run/artifacts-fresh.path)" s11-ub-a 4872 18872
