#!/bin/bash
# set15: after the build: the read-only proof of every archive of both documents, the archive pins (host side, read
# from the tarballs), the offline tests in copy b, and the dry run of every cell (no lab is created).
source <redacted>
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
H=$R/harness-b/deploy/e2e/release-recovery
for d in update fresh; do
  ART=$(cat $R/artifacts-$d.path)
  timeout 900 python3 $H/set15_fixture.py prove --artifacts "$ART" > $R/build/$d-prove.json 2> $R/build/$d-prove.stderr.txt; echo "prove $d rc=$?"
done
bash $J/pins.sh > $R/build/archive-pins.txt 2>&1; echo "pins rc=$?"; cat $R/build/archive-pins.txt | grep -v '^#' | head -40
( cd $H && SET12_RUN_ROOT=$R/test-root timeout 600 python3 -m unittest -v test_set15_trial test_set14_trial test_set12_trial ) > $L/suite-b.txt 2>&1; echo "suite rc=$?"; tail -n 4 $L/suite-b.txt
ls $R/test-root 2>/dev/null && echo "(test-root above: should not exist; tests use a temporary directory)"
for c in set15-debian13 set15-ubuntu set15-arch; do
  timeout 300 bash $H/run-set15.sh dry-run $c "$(cat $R/artifacts-fresh.path)" s15-dry > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$? steps=$(python3 -I -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["steps"]))' $R/build/dry-$c.json 2>&1)"
done
for c in upd1-debian13-good upd1-debian13-defective; do
  timeout 300 bash $H/run-set15.sh dry-run $c "$(cat $R/artifacts-update.path)" s15-dry > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$? kind=$(python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]));print(d["cell_kind"], d["baseline_ref"], len(d["update"].get("steps") or []))' $R/build/dry-$c.json 2>&1)"
done
ls -d /var/tmp/cp-release-drill-* 2>/dev/null || echo "no lab directory exists"
