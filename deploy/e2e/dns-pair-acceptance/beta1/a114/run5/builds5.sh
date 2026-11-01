#!/bin/bash
# run 5: candidate dist (budget gate is fatal), the lab-only 115 label of the same candidate, the alpha.113 tools tree. Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mkdir -p /var/tmp/a114r5; ls -la /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go || exit 1
date -u +%T
bash $H/build.sh lab/alpha114-label-r5 v0.1.0-alpha.114 > /var/tmp/a114r5/out-build-114.out 2>&1
rc=$?; echo "build 114 exit $rc"; date -u +%T
tail -25 /var/tmp/a114r5/out-build-114.out
[ $rc = 0 ] || exit $rc
grep -q '^EXIT 0' /var/tmp/a114r5/out-build-114.out || { echo "114 dist build failed"; exit 3; }
bash $H/build115.sh lab/alpha114-label115-r5 v0.1.0-alpha.115 > /var/tmp/a114r5/out-build-115.out 2>&1
echo "build 115 exit $?"; date -u +%T; tail -6 /var/tmp/a114r5/out-build-115.out
C113=d3f3a323da46ce8f8dca5ad4385666c5428c353e
R=/var/tmp/a114r5/repo-113; rm -rf $R; git init -q $R && git -c safe.directory='*' -C $R fetch -q "$H/src.bundle" refs/heads/lab/alpha113-release:refs/heads/rel
T=/var/tmp/a114r5/tools-$C113; rm -rf $T; mkdir -p $T; git -c safe.directory='*' -C $R archive $C113 deploy/e2e/dns-pair-acceptance download-portal | tar -x -C $T
rm -rf $R; ls $T/download-portal | head -3
echo BUILDS-DONE
