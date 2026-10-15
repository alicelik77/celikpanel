#!/bin/bash
# set17b: after run a: the collection reading starts with set +e: run copy e (every set17/set17b harness
# file, the helper triggered by completion.pending), the offline checks in copy d, the dry runs from copy d.
source <redacted>
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
bash $J/mkov.sh e deploy/e2e/release-recovery/set17_fixture.py deploy/e2e/release-recovery/set17_trial.py deploy/e2e/release-recovery/build-set17-artifacts.sh deploy/e2e/release-recovery/run-set17.sh web/tools/browser-inspect/cold-load-set17.mjs deploy/e2e/release-recovery/set17b_trial.py deploy/e2e/release-recovery/run-set17b.sh deploy/e2e/release-recovery/guest_set17b_restart_delay.py web/tools/browser-inspect/cold-load-set17b.mjs > $L/mkov-e.txt 2>&1; echo "mkov d rc=$?"; tail -n 10 $L/mkov-e.txt
H=$R/harness-e/deploy/e2e/release-recovery
bash $J/offline.sh $H > $L/suite-e.txt 2>&1; echo "suite rc=$?"; cat $L/suite-e.txt
timeout 300 bash $H/run-set17b.sh dry-run set17b-upd86-debian13-good "$(cat $R/artifacts-second.path)" s17b-dry > $R/build/dry-e-set17b-upd86-debian13-good.json 2> $R/build/dry-e-set17b-upd86-debian13-good.err; echo "dry upd86 rc=$?"
timeout 300 bash $H/run-set17b.sh dry-run set17b-upd1-debian13-good-control "$(cat $R/artifacts-update.path)" s17b-dry > $R/build/dry-e-set17b-upd1-debian13-good-control.json 2> $R/build/dry-e-set17b-upd1-debian13-good-control.err; echo "dry control rc=$?"
echo "$(date -u +%FT%TZ) run copy e made after run a (the delay step reading starts with set +e)" >> $R/progress.txt
