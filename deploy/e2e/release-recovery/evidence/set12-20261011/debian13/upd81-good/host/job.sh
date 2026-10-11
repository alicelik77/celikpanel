#!/bin/bash
# set12: one cell on one new lab. Environment from the caller's wrapper: CELL ART LAB PORT LPORT COPY [HAND]
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set12-run
export SET12_NEIGHBOUR_GATE=<scratchpad>/set12/tools/neighbour.sh
export SET12_DISK_GATE=<scratchpad>/set12/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
exec bash /var/tmp/cp-set12-run/harness-${SET12_COPY:-c}/deploy/e2e/release-recovery/run-set12.sh cell upd1-debian13-good "$(cat /var/tmp/cp-set12-run/artifacts-a81.path)" s12-g81-d13 4971 18971
