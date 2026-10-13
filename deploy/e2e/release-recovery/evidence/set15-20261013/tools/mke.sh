#!/bin/bash
# set15: run copy e = copy a's archive + the six set15 harness files as they are now (set15_trial.py uploads the WHOLE guest helper chain
# of guest_set14_native.py in the update cells; a test checks it). Then the offline tests in copy d.
source <redacted>
bash $J/mkov.sh e deploy/e2e/release-recovery/set15_fixture.py deploy/e2e/release-recovery/set15_trial.py deploy/e2e/release-recovery/build-set15-artifacts.sh deploy/e2e/release-recovery/run-set15.sh deploy/e2e/release-recovery/test_set15_trial.py web/tools/browser-inspect/cold-load-set15.mjs > $L/mkov-e.txt 2>&1; echo "mkov e rc=$?"; cat $L/mkov-e.txt
diff $R/overlay-d/files.sha256 $R/overlay-e/files.sha256
H=$R/harness-e/deploy/e2e/release-recovery
( cd $H && SET12_RUN_ROOT=$R/test-root timeout 600 python3 -m unittest -v test_set15_trial test_set14_trial test_set12_trial ) > $L/suite-e.txt 2>&1; echo "suite rc=$?"; grep -E "^(ERROR|FAIL|Ran|OK|FAILED)" $L/suite-e.txt | head
timeout 300 bash $H/run-set15.sh dry-run upd1-debian13-good "$(cat $R/artifacts-update.path)" s15-dry > $R/build/dry-e-upd1-debian13-good.json 2> $R/build/dry-e-upd1-debian13-good.err; echo "dry rc=$?"
