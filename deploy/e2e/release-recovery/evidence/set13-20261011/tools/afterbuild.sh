#!/bin/bash
# set13, after the build: the artifacts document proven read-only on the host, the dry runs of the cells (no lab).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set13-run
R=/var/tmp/cp-set13-run; L=$R/logs
H=$R/harness-${1:-b}/deploy/e2e/release-recovery
FR=$(tail -n 2 $L/build-fresh.out | grep -m1 upd1-artifacts.json); echo "$FR" > $R/artifacts-fresh.path
bash $H/run-upd1.sh prove "$FR" > $R/build/fresh-prove.json 2> $R/build/fresh-prove.stderr.txt; r=$?
echo "prove fresh rc=$r"; echo "$(date -u +%FT%TZ) prove fresh rc=$r" >> $R/progress.txt
for c in set13-debian13 set13-ubuntu set13-arch; do
  bash $H/run-set13.sh dry-run $c "$FR" dry-$c > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$?"; tail -n 2 $R/build/dry-$c.err; done
python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print({k: (d[k]['version'], d[k]['commit'][:12], d[k]['sha256'][:16]) for k in ('baseline','good','defective') if k in d})" "$FR"
ls -d /var/tmp/cp-release-drill-dry-* 2>/dev/null || echo "no dry-run lab was created"
