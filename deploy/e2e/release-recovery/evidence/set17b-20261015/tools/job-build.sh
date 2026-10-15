#!/bin/bash
# set17b build, from run copy b (set17's build script, unchanged): the update document (B = alpha.85 tag commit,
# G = the source ecd2a5f08 itself (its own policy 86), D = G + defect), then the second document (B = that G, dist
# reused by name; G/D labelled v0.1.0-alpha.87).
export PYTHONDONTWRITEBYTECODE=1
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET12_RUN_ROOT=/var/tmp/cp-set17b
export SET17_BUILD_ROOT=/var/tmp/cp-set17b/cp-upd1-build
H=/var/tmp/cp-set17b/harness-b/deploy/e2e/release-recovery
L=/var/tmp/cp-set17b/logs
go version
echo "=== update $(date -u +%FT%TZ)"
bash $H/build-set17-artifacts.sh --baseline-ref v0.1.0-alpha.85 ecd2a5f088d676f7cc8a8e571ed3c903ea951f3e > $L/build-update.out 2> $L/build-update.err; rc1=$?
echo "update rc=$rc1 $(tail -n 2 $L/build-update.out | tr '\n' ' ')"
[ $rc1 -eq 0 ] || exit 1
grep -m1 'upd1-artifacts.json' $L/build-update.out > /var/tmp/cp-set17b/artifacts-update.path
good=$(python3 -I -c 'import json,sys;print(json.load(open(sys.argv[1]))["good"]["commit"])' "$(cat /var/tmp/cp-set17b/artifacts-update.path)")
echo "=== second $(date -u +%FT%TZ) installed=$good"
UPD1_REUSE_DIST_OF_COMMIT=$good bash $H/build-set17-artifacts.sh --second "$(cat /var/tmp/cp-set17b/artifacts-update.path)" > $L/build-second.out 2> $L/build-second.err; rc2=$?
echo "second rc=$rc2 $(tail -n 2 $L/build-second.out | tr '\n' ' ')"
[ $rc2 -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-second.out > /var/tmp/cp-set17b/artifacts-second.path
echo "=== end $(date -u +%FT%TZ)"
[ $rc2 -eq 0 ]
