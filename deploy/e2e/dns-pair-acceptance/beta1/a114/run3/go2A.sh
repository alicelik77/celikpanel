#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
bash $H/waitf.sh pairA.out "pair done" 3600 > /dev/null
timeout 7200 bash $H/opsA.sh > /var/tmp/a114r3/opsA.out 2>&1
rc=$?; echo "A ops exit $rc"; [ $rc = 0 ] || exit $rc
timeout 2400 bash $H/shotsA.sh > /var/tmp/a114r3/shotsA.out 2>&1
echo "A shots exit $?"
