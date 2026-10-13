#!/bin/bash
# set14b: one cell on one new lab (set14b-debian13), with the browser window (SET12_HAND=h14bd13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set14b-run
export SET14B_NEIGHBOUR_GATE=<redacted>
export SET14B_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h14bd13
exec bash /var/tmp/cp-set14b-run/harness-b/deploy/e2e/release-recovery/run-set14b.sh cell set14b-debian13 "$(cat /var/tmp/cp-set14b-run/artifacts-fresh.path)" s14b-d13-a 5151 19151
