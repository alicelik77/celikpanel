#!/bin/bash
# run 4 track N: a fresh candidate Arch server with mail (get.sh; S-new-4 screens), then opsN4.sh. Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4
source $H/env.sh
( export CELL=a6-n IP=192.0.3.183 Z=ar114.test VER=v0.1.0-alpha.114 MEM=2304 TRUST=1 SHOTS=1 MAIN_SITE=here
  timeout 5400 bash $H/single3.sh > /var/tmp/a114r4/singleN.out 2>&1; echo "single exit $?" )
tail -n 10 /var/tmp/a114r4/singleN.out | cut -c1-300
grep -q '^succeeded' <(opnow a6-n main) || { echo "STOP: the candidate Arch setup did not succeed"; exit 6; }
timeout 9000 bash $H/opsN4.sh > /var/tmp/a114r4/opsN.out 2>&1
rc=$?; echo "N ops exit $rc"; tail -n 40 /var/tmp/a114r4/opsN.out | cut -c1-1500
exit $rc
