#!/bin/bash
# set15: one cell on one new lab (upd1-debian13-good), with the browser window (SET12_HAND=h15ugoodc).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set15-run
export SET15_NEIGHBOUR_GATE=<redacted>
export SET15_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h15ugoodc
exec bash /var/tmp/cp-set15-run/harness-e/deploy/e2e/release-recovery/run-set15.sh cell upd1-debian13-good "$(cat /var/tmp/cp-set15-run/artifacts-update.path)" s15-u83-good-c 5167 19167
