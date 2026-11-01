#!/bin/bash
# run 4, R continued after the H2 harness correction: E-new-2(a) on the 113 -> candidate Arch server. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=a6-r; IP=192.0.3.182; Z=ar113.test; S=shopr5.test
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL' | cut -c1-4000; }
exec > >(tee -a $LOGS/opsR.log) 2>&1
echo "== H2 journal after the update (read only) $(ts)"
bash $H/gq.sh $CELL main "journalctl --no-pager -o short-iso --since -30min | grep -E 'finishing another server operation|agent_idle|Quiesce abort|fail-closed|state=unchanged|celikpanel-panel.service: (Main process exited|Failed with result)|Stopped celikpanel|Started celikpanel' | grep -v level=debug | cut -c1-260 | tail -30; systemctl --failed --no-legend | cut -c1-120"
echo "== E-new-2(a): first autoresponder save on the updated Arch server $(ts)"
bash $H/gq.sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; systemctl show dovecot -p ActiveEnterTimestamp"
OWN $CELL main autoresp_first $S
T2=$(b1 sh $CELL main "date +%s" --label now --limit 100 | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].strip())')
echo "-- the next save (and mail: 2 delivered, 1 reply, no bounce) $(ts)"
OWN $CELL main autoresp_e4 $S mail.$Z basic
OWN $CELL main reload_clean $T2
echo "== R done $(ts)"
