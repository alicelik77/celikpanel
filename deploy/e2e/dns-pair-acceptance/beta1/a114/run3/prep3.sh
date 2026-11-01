#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r3/shot.js
bash $H/shotsetup.sh 2>&1 | tail -3
lab ssh beta1-lab labnet -- "python3 -c 'import dkim; print(\"dkimpy ok\")'; date -u; ls /root | head -30"
for f in mailtest.py webmailtest.py dkimcheck.py mailx.py; do lab ssh beta1-lab labnet -- "sudo -n tee /root/$f >/dev/null" < $H/$f && echo "uploaded $f"; done
bash $H/portal.sh $C114 2>&1 | tail -4
bash $H/portal.sh $C113 2>&1 | tail -4
ls /var/tmp/a114r3/
ls /var/tmp/cp-beta1/state 2>&1 | head; ls /var/tmp/cp-beta1/lab/cells 2>&1 | head
free -g
