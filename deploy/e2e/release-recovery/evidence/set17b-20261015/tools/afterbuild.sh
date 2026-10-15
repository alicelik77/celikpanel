#!/bin/bash
# set17b: after the build: run copy c (every set17/set17b harness file), the read-only proof of every archive of both
# documents, the archive pins, the offline checks in copy c, and the dry run of every set17b cell (no lab is created).
source <redacted>
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
bash $J/mkov.sh c deploy/e2e/release-recovery/set17_fixture.py deploy/e2e/release-recovery/set17_trial.py deploy/e2e/release-recovery/build-set17-artifacts.sh deploy/e2e/release-recovery/run-set17.sh web/tools/browser-inspect/cold-load-set17.mjs deploy/e2e/release-recovery/set17b_trial.py deploy/e2e/release-recovery/run-set17b.sh deploy/e2e/release-recovery/guest_set17b_restart_delay.py web/tools/browser-inspect/cold-load-set17b.mjs > $L/mkov-c.txt 2>&1; echo "mkov c rc=$?"; tail -n 10 $L/mkov-c.txt
H=$R/harness-c/deploy/e2e/release-recovery
for d in update second; do
  ART=$(cat $R/artifacts-$d.path)
  timeout 900 python3 $H/set17_fixture.py prove --artifacts "$ART" > $R/build/$d-prove.json 2> $R/build/$d-prove.stderr.txt; echo "prove $d rc=$?"
done
bash $J/pins.sh > $R/build/archive-pins.txt 2>&1; echo "pins rc=$?"; grep -E '^###|version=|current=|previous_commit|bootstrap_release_(sequence|version)' $R/build/archive-pins.txt | head -40
bash $J/offline.sh $H > $L/suite-c.txt 2>&1; echo "suite rc=$?"; cat $L/suite-c.txt
timeout 300 bash $H/run-set17b.sh dry-run set17b-upd86-debian13-good "$(cat $R/artifacts-second.path)" s17b-dry > $R/build/dry-set17b-upd86-debian13-good.json 2> $R/build/dry-set17b-upd86-debian13-good.err; echo "dry upd86 rc=$? $(tail -c 300 $R/build/dry-set17b-upd86-debian13-good.err)"
timeout 300 bash $H/run-set17b.sh dry-run set17b-upd1-debian13-good-control "$(cat $R/artifacts-update.path)" s17b-dry > $R/build/dry-set17b-upd1-debian13-good-control.json 2> $R/build/dry-set17b-upd1-debian13-good-control.err; echo "dry control rc=$? $(tail -c 300 $R/build/dry-set17b-upd1-debian13-good-control.err)"
ls -d /var/tmp/cp-release-drill-* 2>/dev/null || echo "no lab directory exists"
