#!/bin/bash
# set13 build, from run copy a (git archive 816b8dd60; no harness file of the working tree):
#  fresh: `run-upd1.sh build 816b8dd60` (default mode): B = 816b8dd60 labelled v0.1.0-alpha.81 / 81 with the
#         acceptance-test licence build tag, installed fresh by the set13 cells. G, D, S, R are built because the
#         artifacts document needs them and are never installed.
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
H=/var/tmp/cp-set13-run/harness-a/deploy/e2e/release-recovery
L=/var/tmp/cp-set13-run/logs
go version
echo "=== fresh $(date -u +%FT%TZ)"
bash $H/run-upd1.sh build 816b8dd60 > $L/build-fresh.out 2> $L/build-fresh.err; rc1=$?
echo "fresh rc=$rc1 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
echo "$(date -u +%FT%TZ) build fresh rc=$rc1" >> /var/tmp/cp-set13-run/progress.txt
echo "=== end $(date -u +%FT%TZ)"
[ $rc1 -eq 0 ]
