#!/bin/bash
# run 3 phase 1: lab portal on labnet (latest alpha.113), topology A (candidate) and the alpha.113 Arch server in parallel.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
source $H/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r3/shot.js
bash $H/portalnet.sh up v0.1.0-alpha.113 > /var/tmp/a114r3/portalnet-up.out 2>&1; tail -3 /var/tmp/a114r3/portalnet-up.out
( export CELL=a5-a MIP=192.0.3.180 BIP=192.0.3.181 Z=fa5.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.114 BVER=v0.1.0-alpha.114 MKIND=pair-main BKIND=pair-backup-mail MMEM=3072 BMEM=2560 SHOTS=0 MAIN_SITE=here MNAME=frankfurt BNAME=boston
  timeout 5400 bash $H/pair2.sh > /var/tmp/a114r3/pairA.out 2>&1; echo "A exit $?" ) &
sleep 30
( export CELL=a5-r IP=192.0.3.182 Z=ar113.test VER=v0.1.0-alpha.113 MEM=2560 TRUST=1 SHOTS=0 MAIN_SITE=none
  timeout 5400 bash $H/single3.sh > /var/tmp/a114r3/singleR.out 2>&1; echo "R exit $?" ) &
wait
echo "== phase 1 done $(ts)"
tail -n 14 /var/tmp/a114r3/pairA.out | cut -c1-300
tail -n 14 /var/tmp/a114r3/singleR.out | cut -c1-300
