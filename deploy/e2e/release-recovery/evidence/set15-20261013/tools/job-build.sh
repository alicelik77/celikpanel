#!/bin/bash
# set15 build, from run copy b (git archive 1636b73f0 + the set15 harness files): the update document
# (build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83: B = the alpha.83 tag commit, G = 1636b73f0 itself,
# D = 1636b73f0 + set5's migrate-only defect), then the fresh document (--fresh: B = 1636b73f0 itself, whose dist the
# first build left and which is reused by name; G/D labelled v0.1.0-alpha.85, served by the origin only).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set15-run
H=/var/tmp/cp-set15-run/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set15-run/logs
go version
echo "=== update $(date -u +%FT%TZ)"
bash $H/build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83 1636b73f0 > $L/build-update.out 2> $L/build-update.err; rc1=$?
echo "update rc=$rc1 $(tail -n 2 $L/build-update.out | tr '\n' ' ')"
[ $rc1 -eq 0 ] || exit 1
grep -m1 'upd1-artifacts.json' $L/build-update.out > /var/tmp/cp-set15-run/artifacts-update.path
echo "=== fresh $(date -u +%FT%TZ)"
UPD1_REUSE_DIST_OF_COMMIT=1636b73f0979d9921ff99f283ceec8cfed1041dc bash $H/build-set15-artifacts.sh --fresh 1636b73f0 > $L/build-fresh.out 2> $L/build-fresh.err; rc2=$?
echo "fresh rc=$rc2 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
[ $rc2 -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-fresh.out > /var/tmp/cp-set15-run/artifacts-fresh.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc2 -eq 0 ]
