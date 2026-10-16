#!/bin/bash
# set19 build: B = the alpha.90 tag, G = $C + policy 92 after 90, D = G + defect (never installed).
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
export PATH=/opt/celikpanel-test-toolchains/go1.26.5/go/bin:$PATH
export SET19_BUILD_ROOT=$R/cp-upd1-build CELIKPANEL_REPO=$R/repo
H=$R/harness/deploy/e2e/release-recovery
echo "=== build $(date -u +%FT%TZ)" > $L/build.start
timeout 3600 bash $H/build-set19-artifacts.sh $C > $L/build-set19.out 2> $L/build-set19.err; rc=$?
echo $rc > $L/build.rc
echo "=== end $(date -u +%FT%TZ) rc=$rc" > $L/build.end
[ $rc -eq 0 ] && grep -m1 'upd1-artifacts.json' $L/build-set19.out > $R/artifacts.path
echo "$(date -u +%FT%TZ) build rc=$rc" >> $R/progress.txt
