#!/bin/bash
# set14c: after the build: the read-only proof of every archive, run copy b (a + the four set14c harness files), the
# offline tests of set14c (and set14's, unchanged) in copy b, and the dry run of the one cell (no lab is created).
source <redacted>
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
ART=$(cat $R/artifacts-fresh.path)
timeout 900 bash $R/harness-a/deploy/e2e/release-recovery/run-upd1.sh prove "$ART" > $R/build/fresh-prove.json 2> $R/build/fresh-prove.stderr.txt; echo "prove rc=$?"
bash $J/mkov.sh b deploy/e2e/release-recovery/set14c_trial.py deploy/e2e/release-recovery/run-set14c.sh deploy/e2e/release-recovery/test_set14c_trial.py web/tools/browser-inspect/cold-load-set14c.mjs > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; cat $L/mkov-b.txt
H=$R/harness-b/deploy/e2e/release-recovery
( cd $H && SET12_RUN_ROOT=$R/test-root timeout 600 python3 -m unittest -v test_set14c_trial test_set14b_trial test_set14_trial ) > $L/suite-b.txt 2>&1; echo "suite rc=$?"; tail -n 4 $L/suite-b.txt
ls $R/test-root 2>/dev/null && echo "(test-root above: should not exist; tests use a temporary directory)"
for c in set14c-debian13; do
  timeout 300 bash $H/run-set14c.sh dry-run $c "$ART" s14c-dry > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$? steps=$(python3 -I -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["steps"]))' $R/build/dry-$c.json 2>&1)"
done
ls -d /var/tmp/cp-release-drill-* 2>/dev/null || echo "no lab directory exists"
