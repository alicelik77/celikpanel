#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
source $H/env.sh
for c in a4-a; do rm -rf /var/tmp/cp-beta1/state/$c; [ -d /var/tmp/cp-beta1/evidence/a4-a ] && mv /var/tmp/cp-beta1/evidence/a4-a /var/tmp/a114r3/attempt1/evidence-a4-a-second; done
timeout 5400 bash $H/pairR3.sh > /var/tmp/a114r3/pairR3.out 2>&1
echo A3-PAIR-DONE
timeout 9000 bash $H/opsA3.sh > /var/tmp/a114r3/opsA3.out 2>&1
echo A3-OPS-DONE
for n in main backup; do bash $H/collnode.sh a4-a $n > /var/tmp/a114r3/coll-a4-a-$n.out 2>&1; done
echo A4-DONE
