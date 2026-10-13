#!/bin/bash
# set15: one cell on one new lab (set15-debian13), with the browser window (SET12_HAND=h15d13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set15-run
export SET15_NEIGHBOUR_GATE=<redacted>
export SET15_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h15d13
exec bash /var/tmp/cp-set15-run/harness-b/deploy/e2e/release-recovery/run-set15.sh cell set15-debian13 "$(cat /var/tmp/cp-set15-run/artifacts-fresh.path)" s15-d13-a 5161 19161
