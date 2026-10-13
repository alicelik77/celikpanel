#!/bin/bash
# set15: run copy c = copy a's archive + the six set15 harness files as they are now (the browser script gained the
# stale Components load after the Debian run a); the driver files are unchanged from copy b.
source <redacted>
bash $J/mkov.sh c deploy/e2e/release-recovery/set15_fixture.py deploy/e2e/release-recovery/set15_trial.py deploy/e2e/release-recovery/build-set15-artifacts.sh deploy/e2e/release-recovery/run-set15.sh deploy/e2e/release-recovery/test_set15_trial.py web/tools/browser-inspect/cold-load-set15.mjs > $L/mkov-c.txt 2>&1; echo "mkov c rc=$?"; cat $L/mkov-c.txt
diff $R/overlay-b/files.sha256 $R/overlay-c/files.sha256
