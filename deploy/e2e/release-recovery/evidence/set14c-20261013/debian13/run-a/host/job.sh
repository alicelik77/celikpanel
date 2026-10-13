#!/bin/bash
# set14c: one cell on one new lab (set14c-debian13), with the browser window (SET12_HAND=h14cd13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set14c-run
export SET14C_NEIGHBOUR_GATE=<redacted>
export SET14C_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h14cd13
exec bash /var/tmp/cp-set14c-run/harness-b/deploy/e2e/release-recovery/run-set14c.sh cell set14c-debian13 "$(cat /var/tmp/cp-set14c-run/artifacts-fresh.path)" s14c-d13-a 5152 19152
