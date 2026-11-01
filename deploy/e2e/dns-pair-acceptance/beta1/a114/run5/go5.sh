#!/bin/bash
# run 5 phase 1: harness to the lab host, screenshot tool, labnet mail helpers, signed lab portals (114 candidate,
# 113, 115-lab), the network lab portal (latest 114), then topology A on the candidate: Debian 13 first server
# (PowerDNS main, web, mail, DNS) + Ubuntu 24.04 joined by code. Lab only.
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5
source $H/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r5/shot.js
bash $H/shotsetup.sh 2>&1 | tail -3
for f in mailtest.py webmailtest.py dkimcheck.py mailx.py; do lab ssh beta1-lab labnet -- "sudo -n tee /root/$f >/dev/null" < $H/$f && echo "uploaded $f"; done
for c in $C114 $C113 $C115; do bash $H/portal.sh $c > /var/tmp/a114r5/portal-$c.out 2>&1; echo "portal $c exit $?"; done
bash $H/portalnet.sh up v0.1.0-alpha.114 > /var/tmp/a114r5/portalnet-up.out 2>&1; tail -3 /var/tmp/a114r5/portalnet-up.out
echo "== topology A $(ts)"
export CELL=a7-a MIP=192.0.3.180 BIP=192.0.3.181 Z=fa7.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.114 BVER=v0.1.0-alpha.114 MKIND=pair-main BKIND=pair-backup-mail MMEM=3072 BMEM=2560 SHOTS=0 MAIN_SITE=here MNAME=frankfurt BNAME=boston
timeout 5400 bash $H/pair2.sh > /var/tmp/a114r5/pairA.out 2>&1; echo "A exit $?"
tail -n 20 /var/tmp/a114r5/pairA.out | cut -c1-300
echo "== phase 1 done $(ts)"
