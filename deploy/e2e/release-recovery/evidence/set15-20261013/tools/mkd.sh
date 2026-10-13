#!/bin/bash
# set15: run copy d = copy a's archive + the six set15 harness files as they are now (set15_trial.py uploads the guest helper chain
# of guest_set14_native.py in the update cells; a test checks it). Then the offline tests in copy d.
source <redacted>
bash $J/mkov.sh d deploy/e2e/release-recovery/set15_fixture.py deploy/e2e/release-recovery/set15_trial.py deploy/e2e/release-recovery/build-set15-artifacts.sh deploy/e2e/release-recovery/run-set15.sh deploy/e2e/release-recovery/test_set15_trial.py web/tools/browser-inspect/cold-load-set15.mjs > $L/mkov-d.txt 2>&1; echo "mkov c rc=$?"; cat $L/mkov-d.txt
diff $R/overlay-c/files.sha256 $R/overlay-d/files.sha256
H=$R/harness-d/deploy/e2e/release-recovery
( cd $H && SET12_RUN_ROOT=$R/test-root timeout 600 python3 -m unittest -v test_set15_trial test_set14_trial test_set12_trial ) > $L/suite-d.txt 2>&1; echo "suite rc=$?"; grep -E "^(ERROR|FAIL|Ran|OK|FAILED)" $L/suite-d.txt | head
timeout 300 bash $H/run-set15.sh dry-run upd1-debian13-good "$(cat $R/artifacts-update.path)" s15-dry > $R/build/dry-d-upd1-debian13-good.json 2> $R/build/dry-d-upd1-debian13-good.err; echo "dry rc=$?"
