#!/bin/bash
# snapB.sh CELL NODE PREFIX : the setup page now (EN/TR, desktop/phone)
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
python3 -I $H/shots.py $1 $2 $3 '[{"name":"setup-progress","path":"/setup","wait":3000}]' ${4:-}
