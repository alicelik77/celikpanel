#!/bin/bash
# run 4 track R: an alpha.113 Arch server with mail (get.sh), then H2 and E-new-2(a) (opsR4.sh). Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4
source $H/env.sh
( export CELL=a6-r IP=192.0.3.182 Z=ar113.test VER=v0.1.0-alpha.113 MEM=2304 TRUST=1 SHOTS=0 MAIN_SITE=none
  timeout 5400 bash $H/single3.sh > /var/tmp/a114r4/singleR.out 2>&1; echo "single exit $?" )
tail -n 10 /var/tmp/a114r4/singleR.out | cut -c1-300
grep -q '^succeeded' <(source $H/env.sh; opnow a6-r main) || { echo "STOP: the alpha.113 Arch setup did not succeed"; touch /var/tmp/a114r4/R-UPDATES-DONE; exit 6; }
timeout 7200 bash $H/opsR4.sh > /var/tmp/a114r4/opsR.out 2>&1
rc=$?; echo "R ops exit $rc"; tail -n 40 /var/tmp/a114r4/opsR.out | cut -c1-1200
exit $rc
