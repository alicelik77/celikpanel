#!/bin/bash
# set16: one cell on one new lab (set16-debian13), with the browser window (SET12_HAND=h16d13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set16
export SET16_NEIGHBOUR_GATE=<redacted>
export SET16_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h16d13
exec bash /var/tmp/cp-set16/harness-c/deploy/e2e/release-recovery/run-set16.sh cell set16-debian13 "$(cat /var/tmp/cp-set16/artifacts-fresh.path)" s16-d13-a 5261 19261
