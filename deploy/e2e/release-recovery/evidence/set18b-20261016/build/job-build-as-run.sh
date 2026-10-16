#!/bin/bash
# set18b build, from run copy b: B = HEAD + policy 87 (installed), G = B + policy 88, D = G + defect (never installed).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set18b
export SET18B_BUILD_ROOT=/var/tmp/cp-set18b/cp-upd1-build
H=/var/tmp/cp-set18b/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set18b/logs
go version
echo "=== build $(date -u +%FT%TZ)"
bash $H/build-set18b-artifacts.sh 6b363ac4680146f856642cffdab0b57662f6159c > $L/build-set18b.out 2> $L/build-set18b.err; rc=$?
echo "build rc=$rc $(tail -n 2 $L/build-set18b.out | tr '\n' ' ')"
[ $rc -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-set18.out > /var/tmp/cp-set18b/artifacts.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc -eq 0 ]
