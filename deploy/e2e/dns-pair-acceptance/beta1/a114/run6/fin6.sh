#!/bin/bash
# run 6 end: collection (redacted, secret-scanned), then the folder scan. Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6
bash $H/collect6.sh "a7-a:main a7-a:backup" 2>&1 | tail -30
bash $H/scanE.sh 2>&1 | tail -4
