#!/bin/bash
# alpha.118 run 2 phase 1: harness sync, candidate dist, labnet infra-up, lab portal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
bash $A/sync.sh; ls $L | head -3
echo "start $(ts)"
bash $A/build.sh refs/heads/release/alpha118 v0.1.0-alpha.118 > $B/out-build118.out 2>&1 &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"; tail -2 $B/infra-up.out
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "build done $(ts)"; cat $B/out-build118.out
bash $A/portal.sh $C118 > $B/portal118.out 2>&1; echo "portal118 exit $?"; head -4 $B/portal118.out; grep -E "^(version|commit|sequence)=" $B/portal118.out
echo "phase1 done $(ts)"
