#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2
bash $H/webp.sh 2>&1 | tail -4
bash $H/collect2.sh "a4-a:main a4-a:backup" 2>&1 | tail -25
