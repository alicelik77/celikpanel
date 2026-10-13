#!/bin/bash
# set14: one cell on one new lab (set14-ubuntu), with the browser window (SET12_HAND=h14ub).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set14-run
export SET14_NEIGHBOUR_GATE=<redacted>
export SET14_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h14ub
exec bash /var/tmp/cp-set14-run/harness-b/deploy/e2e/release-recovery/run-set14.sh cell set14-ubuntu "$(cat /var/tmp/cp-set14-run/artifacts-fresh.path)" s14-ub-a 5142 19142
