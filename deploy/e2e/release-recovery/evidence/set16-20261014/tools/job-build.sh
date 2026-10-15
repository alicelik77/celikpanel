#!/bin/bash
# set16 build, from run copy b: the update document (B = alpha.84 tag commit, G = source + alpha.85 policy, D = G + defect),
# then the fresh document (B = that G, dist reused by name; G/D labelled v0.1.0-alpha.86, served by the origin only).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set16
H=/var/tmp/cp-set16/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set16/logs
go version
echo "=== update $(date -u +%FT%TZ)"
bash $H/build-set16-artifacts.sh --baseline-ref v0.1.0-alpha.84 89cf883971800c39c72c9f8358c58d56aace4194 > $L/build-update.out 2> $L/build-update.err; rc1=$?
echo "update rc=$rc1 $(tail -n 2 $L/build-update.out | tr '\n' ' ')"
[ $rc1 -eq 0 ] || exit 1
grep -m1 'upd1-artifacts.json' $L/build-update.out > /var/tmp/cp-set16/artifacts-update.path
good=$(python3 -I -c 'import json,sys;print(json.load(open(sys.argv[1]))["good"]["commit"])' "$(cat /var/tmp/cp-set16/artifacts-update.path)")
echo "=== fresh $(date -u +%FT%TZ) installed=$good"
UPD1_REUSE_DIST_OF_COMMIT=$good bash $H/build-set16-artifacts.sh --fresh "$(cat /var/tmp/cp-set16/artifacts-update.path)" > $L/build-fresh.out 2> $L/build-fresh.err; rc2=$?
echo "fresh rc=$rc2 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
[ $rc2 -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-fresh.out > /var/tmp/cp-set16/artifacts-fresh.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc2 -eq 0 ]
