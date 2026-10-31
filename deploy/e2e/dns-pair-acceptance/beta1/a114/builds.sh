#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab
mkdir -p /var/tmp/a114r1; ls -la /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go || exit 1
bash $H/build.sh lab/alpha114-label v0.1.0-alpha.114 > /var/tmp/a114r1/out-build-114.out 2>&1
bash $H/build.sh lab/alpha113-release v0.1.0-alpha.113 > /var/tmp/a114r1/out-build-113.out 2>&1
echo BUILDS-DONE
tail -4 /var/tmp/a114r1/out-build-114.out /var/tmp/a114r1/out-build-113.out
