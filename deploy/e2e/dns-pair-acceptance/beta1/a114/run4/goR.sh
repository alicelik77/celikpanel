#!/bin/bash
# run 2 after the host reboot: A again (main first, E1 forced, check 8), the remaining A items, then C/D.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4
source $H/env.sh

timeout 5400 bash $H/pairR2.sh >> /var/tmp/a114r4/pairR.out 2>&1
echo R-PAIR-DONE
timeout 9000 bash $H/opsA3.sh > /var/tmp/a114r4/opsA3.out 2>&1
echo R-OPS-DONE
for n in main backup; do bash $H/collnode.sh a4-a $n > /var/tmp/a114r4/coll-a4-a-$n.out 2>&1; lab stop a4-a $n | tr -d '\n '; echo; done
[ "${SKIP_CD:-0}" = 1 ] && exit 0
timeout 12000 bash $H/cdx.sh > /var/tmp/a114r4/cd.out 2>&1
echo CD-DONE
