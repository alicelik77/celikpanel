#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5/env.sh
bash $H/portalnet.sh down | tail -1
for c in a7-a:main a7-a:backup; do lab stop ${c%%:*} ${c##*:} | tr -d '\n '; echo " stopped $c"; done
lab ssh beta1-lab labnet -- "sudo -n rm -f /root/mailx.py /root/mailtest.py /root/webmailtest.py /root/dkimcheck.py /root/q-a7-a.sh /root/q.sh; echo labnet-clean"
pgrep -af 'qemu' | cut -c1-90
rm -rf /var/tmp/cp-beta1/state/a7-a
rm -rf /var/tmp/cp-beta1/evidence/a7-a
rm -rf /var/tmp/cp-beta1/lab/cells/a7-a
rm -rf /var/tmp/cp-pair-accept/dist/77cee8678725d2b00381f9ba51967d56b6c9a878-acceptance-license
rm -rf /var/tmp/a114r5
ls /var/tmp/cp-beta1/lab/cells /var/tmp/cp-pair-accept/dist
df -h /var/tmp | tail -1
