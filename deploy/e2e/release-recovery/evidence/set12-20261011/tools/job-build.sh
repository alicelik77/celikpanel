#!/bin/bash
# set12 builds, from run copy a (git archive 8fabc03b3; no harness file of the working tree):
#  fresh: `run-upd1.sh build 8fabc03b3` (default mode): B = 8fabc03b3 labelled v0.1.0-alpha.81 / 81 with the
#         acceptance-test licence build tag, installed fresh by the set12 fresh cells. G, D, S, R are built because the
#         artifacts document needs them and are never installed.
#  a81:   `run-upd1.sh build --baseline-ref v0.1.0-alpha.81 8fabc03b3` with UPD1_REUSE_DIST_OF_COMMIT naming the
#         published tag's commit (its acceptance-licence dist left by set3, reused read-only): B = the published
#         alpha.81, G = 8fabc03b3 labelled v0.1.0-alpha.82 / 82, D, S. One good update cell on Debian.
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
H=/var/tmp/cp-set12-run/harness-a/deploy/e2e/release-recovery
L=/var/tmp/cp-set12-run/logs
go version
echo "=== fresh $(date -u +%FT%TZ)"
bash $H/run-upd1.sh build 8fabc03b3 > $L/build-fresh.out 2> $L/build-fresh.err; rc1=$?
echo "fresh rc=$rc1 $(tail -n 2 $L/build-fresh.out | tr '\n' ' ')"
echo "$(date -u +%FT%TZ) build fresh rc=$rc1" >> /var/tmp/cp-set12-run/progress.txt
sleep 2
echo "=== a81 $(date -u +%FT%TZ)"
UPD1_REUSE_DIST_OF_COMMIT=a0beb7263d1f4ca72258f6b306f9111ba4e2a334 bash $H/run-upd1.sh build --baseline-ref v0.1.0-alpha.81 8fabc03b3 > $L/build-a81.out 2> $L/build-a81.err; rc2=$?
echo "a81 rc=$rc2 $(tail -n 2 $L/build-a81.out | tr '\n' ' ')"
echo "$(date -u +%FT%TZ) build a81 rc=$rc2" >> /var/tmp/cp-set12-run/progress.txt
echo "=== end $(date -u +%FT%TZ)"
[ $rc1 -eq 0 ] && [ $rc2 -eq 0 ]
