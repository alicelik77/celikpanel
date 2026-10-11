#!/bin/bash
# set12 build a82, from run copy b (git archive 8fabc03b3 + set12_fixture.py and build-set12-artifacts.sh):
#  `build-set12-artifacts.sh --baseline-ref v0.1.0-alpha.82 8fabc03b3`: B = the published alpha.82 tag commit
#  (unpatched, its own web build), G = 8fabc03b3 labelled v0.1.0-alpha.83 / 83, D = G + set5's migrate-only defect.
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set12-run
H=/var/tmp/cp-set12-run/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set12-run/logs
echo "=== a82 $(date -u +%FT%TZ)"
bash $H/build-set12-artifacts.sh --baseline-ref v0.1.0-alpha.82 8fabc03b3 > $L/build-a82.out 2> $L/build-a82.err; rc=$?
echo "a82 rc=$rc $(tail -n 2 $L/build-a82.out | tr '\n' ' ')"
echo "$(date -u +%FT%TZ) build a82 rc=$rc" >> /var/tmp/cp-set12-run/progress.txt
[ $rc -eq 0 ]
