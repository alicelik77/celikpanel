#!/bin/bash
# run 4 prep: harness to the lab host, screenshot tool, labnet mail helpers, portals (114, 113, the lab 115 label),
# the network lab portal with latest=114. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r4/shot.js
bash $H/shotsetup.sh 2>&1 | tail -3
lab ssh beta1-lab labnet -- "python3 -c 'import dkim; print(\"dkimpy ok\")'; date -u; ls /root | head -30"
for f in mailtest.py webmailtest.py dkimcheck.py mailx.py; do lab ssh beta1-lab labnet -- "sudo -n tee /root/$f >/dev/null" < $H/$f && echo "uploaded $f"; done
bash $H/portal.sh $C114 2>&1 | tail -3
bash $H/portal.sh $C113 2>&1 | tail -3
bash $H/portal.sh $C115 2>&1 | tail -3
cp /var/tmp/a114r4/portal-v0.1.0-alpha.114/get.sh.diff /var/tmp/a114r4/get.sh.diff 2>/dev/null
bash $H/portalnet.sh up v0.1.0-alpha.114 > /var/tmp/a114r4/portalnet-up.out 2>&1; tail -3 /var/tmp/a114r4/portalnet-up.out
ls /var/tmp/a114r4/
