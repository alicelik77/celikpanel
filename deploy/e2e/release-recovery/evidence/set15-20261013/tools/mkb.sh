#!/bin/bash
# set15: run copy b = copy a's archive + the seven set15 harness files; then the offline tests in it.
source <redacted>
export PYTHONDONTWRITEBYTECODE=1
bash $J/mkov.sh b deploy/e2e/release-recovery/set15_fixture.py deploy/e2e/release-recovery/set15_trial.py deploy/e2e/release-recovery/build-set15-artifacts.sh deploy/e2e/release-recovery/run-set15.sh deploy/e2e/release-recovery/test_set15_trial.py web/tools/browser-inspect/cold-load-set15.mjs > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; cat $L/mkov-b.txt
H=$R/harness-b/deploy/e2e/release-recovery
( cd $H && SET12_RUN_ROOT=$R/test-root timeout 600 python3 -m unittest -v test_set15_trial test_set14_trial test_set12_trial ) > $L/suite-b.txt 2>&1; echo "suite rc=$?"; grep -E "^(ERROR|FAIL|Ran|OK|FAILED)" $L/suite-b.txt | head
ls $R/test-root 2>/dev/null && echo "(test-root exists)"
