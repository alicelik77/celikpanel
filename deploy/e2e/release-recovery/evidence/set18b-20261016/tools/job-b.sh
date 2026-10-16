#!/bin/bash
# set18b: one cell on one new lab (set18-debian13), with the browser window "setup" (SET12_HAND=h18ba).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set18b
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h18ba
exec bash /var/tmp/cp-set18b/harness-c/deploy/e2e/release-recovery/run-set18b.sh cell set18-debian13 "$(cat /var/tmp/cp-set18b/artifacts.path)" s18b-a 5320 19320
