#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2
sleep 60
timeout 5400 bash $H/singleB2.sh > /var/tmp/a114r2/archB.out 2>&1
echo B-SETUP-DONE
timeout 7200 bash $H/opsB2.sh > /var/tmp/a114r2/opsB.out 2>&1
echo B-DONE
