#!/bin/bash
# After the host reboot (lab time ~03:47): every guest died. Clean the dead cells, keep attempt-1 evidence,
# bring labnet up fresh. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
bash $H/sync.sh; cp $H/shot.js /var/tmp/a114r2/shot.js
python3 -I $L/lab.py net-up
for c in a4-a:main a4-a:backup a4-e:main a4-e:backup beta1-lab:labnet; do python3 -I $L/lab.py stop ${c%%:*} ${c##*:}; echo; done
mkdir -p /var/tmp/a114r2/attempt1
for c in a4-a a4-e; do [ -d /var/tmp/cp-beta1/evidence/$c ] && mv /var/tmp/cp-beta1/evidence/$c /var/tmp/a114r2/attempt1/evidence-$c; rm -rf /var/tmp/cp-beta1/state/$c /var/tmp/cp-beta1/lab/cells/$c; done
cp /var/tmp/a114r2/opsA.out /var/tmp/a114r2/attempt1/opsA-attempt1.out; cp /var/tmp/a114r2/pairA.out /var/tmp/a114r2/attempt1/pairA-attempt1.out; cp /var/tmp/a114r2/pairE.out /var/tmp/a114r2/attempt1/pairE-attempt1.out
cp -r /var/tmp/a114r2/owner-log /var/tmp/a114r2/attempt1/owner-log
rm -f /var/tmp/a114r2/owner-log/a4-a.jsonl /var/tmp/a114r2/owner-log/a4-a-*
timeout 2700 bash $L/infra-up.sh 2>&1 | tail -6
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim >/dev/null 2>&1; python3 -c 'import dkim; print(\"dkimpy ok\")'"
for f in mailtest.py webmailtest.py dkimcheck.py mailx.py; do lab ssh beta1-lab labnet -- "sudo -n tee /root/$f >/dev/null" < $H/$f && echo "uploaded $f"; done
ls /var/tmp/cp-beta1/lab/cells /var/tmp/cp-beta1/state; free -m | head -2
