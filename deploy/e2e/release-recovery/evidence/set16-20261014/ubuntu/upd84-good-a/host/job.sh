#!/bin/bash
# set16: one cell on one new lab (upd1-ubuntu-good), with the browser window (SET12_HAND=h16ubgood).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set16
export SET16_NEIGHBOUR_GATE=<redacted>
export SET16_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h16ubgood
exec bash /var/tmp/cp-set16/harness-c/deploy/e2e/release-recovery/run-set16.sh cell upd1-ubuntu-good "$(cat /var/tmp/cp-set16/artifacts-update.path)" s16-ub-u84-good-a 5264 19264
