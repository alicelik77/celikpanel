#!/bin/bash
# set17: one cell on one new lab (upd1-ubuntu-good), with the browser windows (SET12_HAND=h17ubgood).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set17
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h17ubgood
exec bash /var/tmp/cp-set17/harness-c/deploy/e2e/release-recovery/run-set17.sh cell upd1-ubuntu-good "$(cat /var/tmp/cp-set17/artifacts-update.path)" s17-ub-u85-good-a 5275 19275
