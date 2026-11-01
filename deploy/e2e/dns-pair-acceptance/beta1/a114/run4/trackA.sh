#!/bin/bash
# run 4 track A: topology A on the candidate (Debian 13 first server: PowerDNS main, web, mail, DNS; Ubuntu 24.04 joins by
# code), then H1, Sanity, E-new-3, E-new-4 a/b/c (opsA4.sh), then screens S-new-1..3 and S23 (shotsA.sh). Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4
source $H/env.sh
( export CELL=a6-a MIP=192.0.3.180 BIP=192.0.3.181 Z=fa5.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.114 BVER=v0.1.0-alpha.114 MKIND=pair-main BKIND=pair-backup-mail MMEM=3072 BMEM=2560 SHOTS=0 MAIN_SITE=here MNAME=frankfurt BNAME=boston
  timeout 5400 bash $H/pair2.sh > /var/tmp/a114r4/pairA.out 2>&1; echo "pair exit $?" )
tail -n 16 /var/tmp/a114r4/pairA.out | cut -c1-300
timeout 9000 bash $H/opsA4.sh > /var/tmp/a114r4/opsA.out 2>&1
rc=$?; echo "A ops exit $rc"; tail -n 30 /var/tmp/a114r4/opsA.out | cut -c1-600
[ $rc = 0 ] || exit $rc
timeout 2400 bash $H/shotsA.sh > /var/tmp/a114r4/shotsA.out 2>&1
echo "A shots exit $?"; tail -n 12 /var/tmp/a114r4/shotsA.out | cut -c1-600
