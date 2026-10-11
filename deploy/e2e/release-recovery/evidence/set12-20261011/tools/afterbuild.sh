#!/bin/bash
# set12, after the builds: the three artifact documents proven read-only on the host, the dry runs of the cells (no lab).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set12-run
R=/var/tmp/cp-set12-run; L=$R/logs
H=$R/harness-${1:-c}/deploy/e2e/release-recovery
FR=$(tail -n 2 $L/build-fresh.out | grep -m1 upd1-artifacts.json); echo "$FR" > $R/artifacts-fresh.path
A81=$(tail -n 2 $L/build-a81.out | grep -m1 upd1-artifacts.json); echo "$A81" > $R/artifacts-a81.path
A82=$(tail -n 2 $L/build-a82.out | grep -m1 upd1-artifacts.json); echo "$A82" > $R/artifacts-a82.path
for n in fresh:$FR a81:$A81; do k=${n%%:*}; f=${n#*:}
  bash $H/run-upd1.sh prove "$f" > $R/build/$k-prove.json 2> $R/build/$k-prove.stderr.txt; r=$?
  echo "prove $k rc=$r"; echo "$(date -u +%FT%TZ) prove $k rc=$r" >> $R/progress.txt; done
python3 $H/set12_fixture.py prove --artifacts "$A82" > $R/build/a82-prove.json 2> $R/build/a82-prove.stderr.txt; r=$?
echo "prove a82 rc=$r"; echo "$(date -u +%FT%TZ) prove a82 rc=$r" >> $R/progress.txt
for c in set12-debian13 set12-ubuntu set12-arch; do
  bash $H/run-set12.sh dry-run $c "$FR" dry-$c > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$?"; tail -n 2 $R/build/dry-$c.err; done
for c in upd1-debian13-good upd1-ubuntu-good upd1-arch-good upd1-debian13-defective upd1-ubuntu-defective upd1-arch-defective; do
  bash $H/run-set12.sh dry-run $c "$A82" dry-82-$c > $R/build/dry-82-$c.json 2> $R/build/dry-82-$c.err; echo "dry a82 $c rc=$?"; tail -n 2 $R/build/dry-82-$c.err; done
bash $H/run-set12.sh dry-run upd1-debian13-good "$A81" dry-81 > $R/build/dry-81-upd1-debian13-good.json 2> $R/build/dry-81.err; echo "dry a81 rc=$?"; tail -n 3 $R/build/dry-81.err
for f in "$FR" "$A81" "$A82"; do python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print((d.get('baseline_ref') or {}).get('ref'), {k: (d[k]['version'], d[k]['commit'][:12], d[k]['sha256'][:16], d[k].get('reused_dist')) for k in ('baseline','good','defective') if k in d})" "$f"; done
ls -d /var/tmp/cp-release-drill-dry-* 2>/dev/null || echo "no dry-run lab was created"
