#!/bin/bash
# set11: cell upd1-debian13-startcheck on one new lab (run copy c; artifacts a81).
export PYTHONDONTWRITEBYTECODE=1
export SET11_NEIGHBOUR_GATE=<scratchpad>/set11/tools/neighbour.sh
export SET11_DISK_GATE=<scratchpad>/set11/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
exec bash /var/tmp/cp-set11-run/harness-${SET11_COPY:-c}/deploy/e2e/release-recovery/run-set11.sh cell upd1-debian13-startcheck "$(cat /var/tmp/cp-set11-run/artifacts-a81.path)" s11-sc-${SET11_RUN:-a} 4874 18874
