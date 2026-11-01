#!/bin/bash
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r3/shot.js
export CELL=a4-a MIP=192.0.3.150 BIP=192.0.3.151 Z=fa114.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.114 BVER=v0.1.0-alpha.114 MKIND=pair-main BKIND=pair-backup-web MMEM=4096 BMEM=3072 SHOTS=1
timeout 7200 bash $H/pairA2.sh > /var/tmp/a114r3/pairA.out 2>&1
echo A-PAIR-DONE
[ "${PAIR_ONLY:-0}" = 1 ] && exit 0
timeout 10800 bash $H/opsA2.sh > /var/tmp/a114r3/opsA.out 2>&1
echo A-DONE
