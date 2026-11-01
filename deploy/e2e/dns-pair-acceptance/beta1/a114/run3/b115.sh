#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
bash $H/build115.sh lab/alpha114-label115-r3 v0.1.0-alpha.115 > /var/tmp/a114r3/out-build-115.out 2>&1
echo "exit $?"; head -3 /var/tmp/a114r3/out-build-115.out; tail -4 /var/tmp/a114r3/out-build-115.out
