#!/bin/bash
# C + D: installs on alpha.113 (the owner's current release), the mixed pair (alpha.113 first server on
# 2083 + fresh alpha.114 joined server on 1923), then the owner's Update to the candidate through the
# panel's own update API, then mail added in place on a joined server (check 3b). Lab only.
set -u
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab
source $H/env.sh
exec > >(tee -a $LOGS/cd1.log) 2>&1
bash $H/sync.sh; bash $H/cpshot.sh
echo "== network lab portal (latest = alpha.113) $(ts)"
bash $H/portalnet.sh up v0.1.0-alpha.113
export TRUST_NODES
( export CELL=a4-c MIP=192.0.3.160 BIP=192.0.3.161 Z=c114.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.113 BVER=v0.1.0-alpha.113 MKIND=pair-main BKIND=pair-backup-web MMEM=2560 BMEM=2048 SHOTS=0 MAIN_SITE=none TRUST_NODES="main backup"
  timeout 5400 bash $H/pair.sh > /var/tmp/a114r1/pairC.out 2>&1 )
echo "== C set up $(ts)"
