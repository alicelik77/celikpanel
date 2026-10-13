#!/bin/bash
# set14c: the offline tests on a temporary LF copy of the release-recovery folder (no evidence), before run copy b.
set -u
T=$(mktemp -d /var/tmp/cp-set14c-run/quick.XXXX)
cd '<redacted> PROJECTS/celikpanel/deploy/e2e/release-recovery'
for f in *.py *.sh; do sed 's/\r$//' "$f" > "$T/$f"; done
cd $T && SET12_RUN_ROOT=$T/root PYTHONDONTWRITEBYTECODE=1 timeout 300 python3 -m unittest test_set14c_trial test_set14b_trial test_set14_trial 2>&1 | tail -n 8
cd / && rm -rf -- "$T"
