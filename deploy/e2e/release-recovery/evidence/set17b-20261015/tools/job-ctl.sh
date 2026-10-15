#!/bin/bash
# set17b: one cell on one new lab (set17b-upd1-debian13-good-control), with the browser window "page" (SET12_HAND=h17bctl).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set17b
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h17bctl
exec bash /var/tmp/cp-set17b/harness-e/deploy/e2e/release-recovery/run-set17b.sh cell set17b-upd1-debian13-good-control "$(cat /var/tmp/cp-set17b/artifacts-update.path)" s17b-u85-ctl 5283 19283
