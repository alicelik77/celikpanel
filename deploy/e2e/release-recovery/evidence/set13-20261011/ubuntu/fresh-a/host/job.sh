#!/bin/bash
# set13: one cell on one new lab (set13-ubuntu).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set13-run
export SET13_NEIGHBOUR_GATE=<scratchpad>/set13/tools/neighbour.sh
export SET13_DISK_GATE=<scratchpad>/set13/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
exec bash /var/tmp/cp-set13-run/harness-${SET13_COPY:-d}/deploy/e2e/release-recovery/run-set13.sh cell set13-ubuntu "$(cat /var/tmp/cp-set13-run/artifacts-fresh.path)" s13-ub-a 5091 19091
