#!/bin/bash
# set15: one cell on one new lab (set15-ubuntu), with the browser window (SET12_HAND=h15ub).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set15-run
export SET15_NEIGHBOUR_GATE=<redacted>
export SET15_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h15ub
exec bash /var/tmp/cp-set15-run/harness-b/deploy/e2e/release-recovery/run-set15.sh cell set15-ubuntu "$(cat /var/tmp/cp-set15-run/artifacts-fresh.path)" s15-ub-a 5162 19162
