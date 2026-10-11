#!/bin/bash
# set13: one cell on one new lab (set13-debian13), run b: C4 and C5 again (run a: C3 before C4 left the vhost row foreign; a harness error in C5 e) and the browser window.
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set13-run
export SET13_NEIGHBOUR_GATE=<scratchpad>/set13/tools/neighbour.sh
export SET13_DISK_GATE=<scratchpad>/set13/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h13d13b
export SET13_SECTIONS=C4-pool-not-read,C5-app-unit
exec bash /var/tmp/cp-set13-run/harness-${SET13_COPY:-e}/deploy/e2e/release-recovery/run-set13.sh cell set13-debian13 "$(cat /var/tmp/cp-set13-run/artifacts-fresh.path)" s13-d13-b 5082 19082
