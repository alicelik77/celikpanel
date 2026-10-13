#!/bin/bash
# set15: the offline tests on a temporary LF copy of the release-recovery folder (no evidence), before run copy b.
set -u
T=$(mktemp -d /var/tmp/cp-set15-run/quick.XXXX)
cd '<redacted> PROJECTS/celikpanel/deploy/e2e/release-recovery'
for f in *.py *.sh; do sed 's/\r$//' "$f" > "$T/$f"; done
cd $T && SET12_RUN_ROOT=$T/root PYTHONDONTWRITEBYTECODE=1 timeout 300 python3 -m unittest test_set15_trial test_set14_trial test_set12_trial 2>&1 | grep -E "^(ERROR|FAIL|Ran|OK|FAILED)" | head -20
cd / && rm -rf -- "$T"
