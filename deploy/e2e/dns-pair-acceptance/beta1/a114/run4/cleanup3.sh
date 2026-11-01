#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
bash $H/portalnet.sh down | tail -2
for c in a6-a:main a6-a:backup a6-r:main; do lab stop ${c%%:*} ${c##*:} | tr -d '\n '; echo " stopped $c"; done
lab ssh beta1-lab labnet -- "sudo -n rm -f /root/mailx.py /root/mailtest.py /root/webmailtest.py /root/dkimcheck.py /root/q-a6-a.sh /root/q-a6-r.sh /root/q.sh; echo labnet-clean"
pgrep -af 'qemu' | cut -c1-90
rm -rf /var/tmp/cp-beta1/state/a6-a /var/tmp/cp-beta1/state/a6-r /var/tmp/cp-beta1/state/a6-n
rm -rf /var/tmp/cp-beta1/evidence/a6-a /var/tmp/cp-beta1/evidence/a6-r /var/tmp/cp-beta1/evidence/a6-n
rm -rf /var/tmp/cp-beta1/lab/cells/a6-a /var/tmp/cp-beta1/lab/cells/a6-r /var/tmp/cp-beta1/lab/cells/a6-n
rm -rf /var/tmp/cp-pair-accept/dist/528c5bdfbfff4c633db2c4728cf4936b70c2082f-acceptance-license
rm -rf /var/tmp/a114r4
ls /var/tmp/cp-beta1/lab/cells /var/tmp/cp-pair-accept/dist
df -h /var/tmp | tail -1
