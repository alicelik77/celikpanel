#!/bin/bash
# set16: after the build: run copy c (every set16 harness file), the read-only proof of every archive of both
# documents, the archive pins, the offline tests in copy c, and the dry run of every cell (no lab is created).
source <redacted>
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
bash $J/mkov.sh c deploy/e2e/release-recovery/set16_fixture.py deploy/e2e/release-recovery/set16_trial.py deploy/e2e/release-recovery/build-set16-artifacts.sh deploy/e2e/release-recovery/run-set16.sh deploy/e2e/release-recovery/test_set16_trial.py web/tools/browser-inspect/cold-load-set16.mjs > $L/mkov-c.txt 2>&1; echo "mkov c rc=$?"; tail -n 7 $L/mkov-c.txt
H=$R/harness-c/deploy/e2e/release-recovery
for d in update fresh; do
  ART=$(cat $R/artifacts-$d.path)
  timeout 900 python3 $H/set16_fixture.py prove --artifacts "$ART" > $R/build/$d-prove.json 2> $R/build/$d-prove.stderr.txt; echo "prove $d rc=$?"
done
bash $J/pins.sh > $R/build/archive-pins.txt 2>&1; echo "pins rc=$?"; grep -v '^#' $R/build/archive-pins.txt | grep -E 'sha256=|version=|current=|previous_commit|bootstrap_release_(sequence|version)' | head -40
bash $J/offline.sh $H > $L/suite-c.txt 2>&1; echo "suite rc=$?"; grep -E "^(Ran|OK|FAILED)|compiled|syntax" $L/suite-c.txt
for c in set16-debian13 set16-ubuntu; do
  timeout 300 bash $H/run-set16.sh dry-run $c "$(cat $R/artifacts-fresh.path)" s16-dry > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$? $(tail -c 300 $R/build/dry-$c.err)"
done
for c in upd1-debian13-good upd1-debian13-defective upd1-ubuntu-good upd1-ubuntu-defective; do
  timeout 300 bash $H/run-set16.sh dry-run $c "$(cat $R/artifacts-update.path)" s16-dry > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$? $(tail -c 300 $R/build/dry-$c.err)"
done
ls -d /var/tmp/cp-release-drill-* 2>/dev/null || echo "no lab directory exists"
