#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r3/shot.js
grep -h 'LAB NOTE' /var/tmp/a114r3/out-build-114.out; tail -3 /var/tmp/a114r3/budget-*.log
bash $H/shotsetup.sh 2>&1 | tail -3
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim >/dev/null 2>&1; python3 -c 'import dkim; print(\"dkimpy ok\")'; date -u; systemctl is-active a114-portal 2>&1; ls /root"
for f in mailtest.py webmailtest.py dkimcheck.py mailx.py; do lab ssh beta1-lab labnet -- "sudo -n tee /root/$f >/dev/null" < $H/$f && echo "uploaded $f"; done
bash $H/portal.sh $C114 2>&1 | tail -4
bash $H/portal.sh $C113 2>&1 | tail -4
ls /var/tmp/a114r3/
python3 -I $L/lab.py --help 2>&1 | head -20
