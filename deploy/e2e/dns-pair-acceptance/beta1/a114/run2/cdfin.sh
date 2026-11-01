#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
exec > >(tee -a $LOGS/cdfin.log) 2>&1
echo "== zone record change again (harness body fixed) $(ts)"
infra delegate zc2114.test ns1.c114.test 192.0.3.160 ns2.c114.test 192.0.3.161 > /dev/null
python3 -I $H/owner.py a4-c main zonecrud zc2114.test 192.0.3.160 192.0.3.161 2>&1 | grep -E '^PASS|^FAIL' | cut -c1-500
infra delegate zd2114.test ns1.d114.test 192.0.3.170 ns2.d114.test 192.0.3.171 > /dev/null
python3 -I $H/owner.py a4-d main zonecrud zd2114.test 192.0.3.170 192.0.3.171 2>&1 | grep -E '^PASS|^FAIL' | cut -c1-500
echo "== v3 evidence after the second update $(ts)"
bash $H/v3.sh a4-c main; bash $H/v3.sh a4-d main
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do bash $H/collnode.sh ${c%%:*} ${c##*:} > /var/tmp/a114r2/coll-${c%%:*}-${c##*:}.out 2>&1; done
bash $H/portalnet.sh down 2>&1 | tail -25 > /var/tmp/a114r2/logs/portalnet-requests.log
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do lab stop ${c%%:*} ${c##*:} | tr -d '\n '; echo; done
echo "== cdfin done $(ts)"
