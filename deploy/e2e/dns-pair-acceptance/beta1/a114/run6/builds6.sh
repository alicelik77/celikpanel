#!/bin/bash
# run 6: candidate dist only (budget gate is fatal). No 115-lab build. Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mkdir -p /var/tmp/a114r6; ls -la /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go || exit 1
date -u +%T
bash $H/build.sh lab/alpha114-label-r6 v0.1.0-alpha.114 > /var/tmp/a114r6/out-build-114.out 2>&1
rc=$?; echo "build 114 exit $rc"; date -u +%T
tail -25 /var/tmp/a114r6/out-build-114.out
[ $rc = 0 ] || exit $rc
grep -q '^EXIT 0' /var/tmp/a114r6/out-build-114.out || { echo "114 dist build failed"; exit 3; }
echo BUILDS-DONE
