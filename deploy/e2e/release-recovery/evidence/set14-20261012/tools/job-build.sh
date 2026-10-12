#!/bin/bash
# set14 build, from run copy a (git archive a5830dd3e): `run-upd1.sh build a5830dd3e` (default mode): B = a5830dd3e
# labelled v0.1.0-alpha.81 / 81 with the acceptance-test licence build tag, installed fresh by the set14 cells.
# G, D, S, R are built because the artifacts document needs them and are never installed.
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set14-run
H=/var/tmp/cp-set14-run/harness-a/deploy/e2e/release-recovery
L=/var/tmp/cp-set14-run/logs
go version
echo "=== fresh $(date -u +%FT%TZ)"
bash $H/run-upd1.sh build a5830dd3e > $L/build-fresh.out 2> $L/build-fresh.err; rc1=$?
echo "fresh rc=$rc1 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
[ $rc1 -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-fresh.out > /var/tmp/cp-set14-run/artifacts-fresh.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc1 -eq 0 ]
