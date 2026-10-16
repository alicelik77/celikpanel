#!/bin/bash
# set19: one cell on one new lab (set19-catchup-debian13), with the browser window "page" (SET12_HAND=h19cb).
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
export SET17_NEIGHBOUR_GATE=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h19cb
echo "$(date -u +%FT%TZ) cell set19-catchup-debian13 start" >> $R/progress.txt
timeout 5400 bash $R/harness/deploy/e2e/release-recovery/run-set19.sh cell set19-catchup-debian13 "$(cat $R/artifacts.path)" s19-catchup-b 5419 19419 > $L/cell-catchup-b.out 2> $L/cell-catchup-b.err
rc=$?
echo "$(date -u +%FT%TZ) cell set19-catchup-debian13 end rc=$rc" >> $R/progress.txt
echo $rc > $L/cell-catchup-b.rc
