#!/bin/bash
# set17: one cell on one new lab (set17-debian13), with the browser windows (SET12_HAND=h17d13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set17
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h17d13
exec bash /var/tmp/cp-set17/harness-c/deploy/e2e/release-recovery/run-set17.sh cell set17-debian13 "$(cat /var/tmp/cp-set17/artifacts-second.path)" s17-d13-a 5271 19271
