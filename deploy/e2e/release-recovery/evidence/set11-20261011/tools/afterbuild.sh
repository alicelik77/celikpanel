#!/bin/bash
# set11, after the builds: both artifact documents proven read-only on the host, the dry runs of the cells (no lab).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
R=/var/tmp/cp-set11-run; L=$R/logs
H=$R/harness-${1:-b}/deploy/e2e/release-recovery
FR=$(tail -n 2 $L/build-fresh.out | grep -m1 upd1-artifacts.json); echo "$FR" > $R/artifacts-fresh.path
A81=$(tail -n 2 $L/build-a81.out | grep -m1 upd1-artifacts.json); echo "$A81" > $R/artifacts-a81.path
for n in fresh:$FR a81:$A81; do k=${n%%:*}; f=${n#*:}
  bash $H/run-upd1.sh prove "$f" > $R/build/$k-prove.json 2> $R/build/$k-prove.stderr.txt; r=$?
  echo "prove $k rc=$r"; echo "$(date -u +%FT%TZ) prove $k rc=$r" >> $R/progress.txt; done
for c in set11-debian13 set11-ubuntu set11-arch; do
  bash $H/run-set11.sh dry-run $c "$FR" dry-$c > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$?"; tail -n 2 $R/build/dry-$c.err; done
bash $H/run-set11.sh dry-run upd1-debian13-startcheck "$A81" dry-sc > $R/build/dry-upd1-debian13-startcheck.json 2> $R/build/dry-sc.err; echo "dry startcheck rc=$?"; tail -n 3 $R/build/dry-sc.err
python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print({k: (d[k]['version'], d[k]['commit'][:12], d[k]['sha256'][:12]) for k in ('baseline','good','defective')})" "$FR"
python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print({k: (d[k]['version'], d[k]['commit'][:12], d[k]['sha256'][:12], d[k].get('reused_dist')) for k in ('baseline','good','defective','startcheck') if k in d})" "$A81"
ls -d /var/tmp/cp-release-drill-dry-* 2>/dev/null || echo "no dry-run lab was created"
