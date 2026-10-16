#!/bin/bash
# set18: one cell on one new lab (set18-debian13), with the browser window "setup" (SET12_HAND=h18b).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set18
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h18b
exec bash /var/tmp/cp-set18/harness-d/deploy/e2e/release-recovery/run-set18.sh cell set18-debian13 "$(cat /var/tmp/cp-set18/artifacts.path)" s18-b 5319 19319
