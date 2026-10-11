#!/bin/bash
# set13: one cell on one new lab (set13-debian13).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set13-run
export SET13_NEIGHBOUR_GATE=<scratchpad>/set13/tools/neighbour.sh
export SET13_DISK_GATE=<scratchpad>/set13/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h13d13
exec bash /var/tmp/cp-set13-run/harness-${SET13_COPY:-b}/deploy/e2e/release-recovery/run-set13.sh cell set13-debian13 "$(cat /var/tmp/cp-set13-run/artifacts-fresh.path)" s13-d13-a 5081 19081
