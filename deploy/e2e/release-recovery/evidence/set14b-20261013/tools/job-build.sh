#!/bin/bash
# set14b build, from run copy a (git archive 1a74eeda5): `run-upd1.sh build 1a74eeda5` (default mode): B = 1a74eeda5
# labelled as the last tag's version with the acceptance-test licence build tag, installed fresh by the set14b cell.
# G, D, S, R are built because the artifacts document needs them and are never installed.
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set14b-run
H=/var/tmp/cp-set14b-run/harness-a/deploy/e2e/release-recovery
L=/var/tmp/cp-set14b-run/logs
go version
echo "=== fresh $(date -u +%FT%TZ)"
bash $H/run-upd1.sh build 1a74eeda5 > $L/build-fresh.out 2> $L/build-fresh.err; rc1=$?
echo "fresh rc=$rc1 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
[ $rc1 -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-fresh.out > /var/tmp/cp-set14b-run/artifacts-fresh.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc1 -eq 0 ]
