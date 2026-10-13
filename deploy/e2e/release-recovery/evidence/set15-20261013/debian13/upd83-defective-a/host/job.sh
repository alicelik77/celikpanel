#!/bin/bash
# set15: one cell on one new lab (upd1-debian13-defective), with the browser window (SET12_HAND=h15udef).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set15-run
export SET15_NEIGHBOUR_GATE=<redacted>
export SET15_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h15udef
exec bash /var/tmp/cp-set15-run/harness-e/deploy/e2e/release-recovery/run-set15.sh cell upd1-debian13-defective "$(cat /var/tmp/cp-set15-run/artifacts-update.path)" s15-u83-def-a 5164 19164
