#!/bin/bash
# set13: one cell on one new lab (set13-ubuntu), run b: C4 again (run a: the pool row still held C2's refused take after the lab put-back; an answered read now comes first).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set13-run
export SET13_NEIGHBOUR_GATE=<scratchpad>/set13/tools/neighbour.sh
export SET13_DISK_GATE=<scratchpad>/set13/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET13_SECTIONS=C4-pool-not-read
exec bash /var/tmp/cp-set13-run/harness-${SET13_COPY:-e}/deploy/e2e/release-recovery/run-set13.sh cell set13-ubuntu "$(cat /var/tmp/cp-set13-run/artifacts-fresh.path)" s13-ub-b 5092 19092
