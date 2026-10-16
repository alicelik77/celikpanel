#!/bin/bash
# set18 build, from run copy b: B = HEAD + policy 87 (installed), G = B + policy 88, D = G + defect (never installed).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set18
export SET18_BUILD_ROOT=/var/tmp/cp-set18/cp-upd1-build
H=/var/tmp/cp-set18/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set18/logs
go version
echo "=== build $(date -u +%FT%TZ)"
bash $H/build-set18-artifacts.sh 5d54aba1e9c93357fa0c19d3b0487e27384acd01 > $L/build-set18.out 2> $L/build-set18.err; rc=$?
echo "build rc=$rc $(tail -n 2 $L/build-set18.out | tr '\n' ' ')"
[ $rc -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-set18.out > /var/tmp/cp-set18/artifacts.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc -eq 0 ]
