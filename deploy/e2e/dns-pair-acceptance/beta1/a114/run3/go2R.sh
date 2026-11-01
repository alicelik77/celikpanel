#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
[ -f /var/tmp/a114r3/opsR.out ] && mv /var/tmp/a114r3/opsR.out /var/tmp/a114r3/opsR-attempt1-hosts-lost.out
SKIP_SITE=1 timeout 5400 bash $H/opsR.sh > /var/tmp/a114r3/opsR.out 2>&1
echo "R ops exit $?"; tail -n 40 /var/tmp/a114r3/opsR.out | cut -c1-900
