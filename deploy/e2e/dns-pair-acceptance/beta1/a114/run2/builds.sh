#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mkdir -p /var/tmp/a114r2; ls -la /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go || exit 1
bash $H/build.sh lab/alpha114-label-r2 v0.1.0-alpha.114 > /var/tmp/a114r2/out-build-114.out 2>&1
# alpha.113: the cached acceptance dist of d3f3a323d is reused; only its tools tree is extracted
C113=d3f3a323da46ce8f8dca5ad4385666c5428c353e
R=/var/tmp/a114r2/repo-113; rm -rf $R; git init -q $R && git -c safe.directory='*' -C $R fetch -q "$H/src.bundle" refs/heads/lab/alpha113-release:refs/heads/rel
T=/var/tmp/a114r2/tools-$C113; rm -rf $T; mkdir -p $T; git -c safe.directory='*' -C $R archive $C113 deploy/e2e/dns-pair-acceptance download-portal | tar -x -C $T
rm -rf $R; ls $T/download-portal | head -3
echo BUILDS-DONE
tail -12 /var/tmp/a114r2/out-build-114.out
