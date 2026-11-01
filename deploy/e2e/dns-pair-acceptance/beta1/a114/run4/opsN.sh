#!/bin/bash
# run 3, a fresh candidate Arch server: S-new-4 (one-server names step), E-new-2(b) (pigeonhole installed with mail,
# autoresponder with no warning, mail + one reply), then E-new-5 on the candidate panel: a lab 115 label of the same
# candidate refused by its installer (unknown panel.env key) -> automatic rollback -> the record; the normal update. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=a6-n; IP=192.0.3.183; Z=ar114.test; S=shopn5.test
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL|SUMMARY-LINE|FINAL-STATUS' | cut -c1-2500; }
ver() { b1 api $1 $2 GET /api/v1/panel/version --label version --limit 4000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", sys.argv[1], b.get("version"), (b.get("commit") or "")[:9], "schema", b.get("schema_version"))' $1/$2; }
( export CELL IP Z VER=v0.1.0-alpha.114 MEM=2560 TRUST=1 SHOTS=1; timeout 5400 bash $H/single3.sh > /var/tmp/a114r4/singleN.out 2>&1 )
exec > >(tee -a $LOGS/opsN.log) 2>&1
tail -n 8 /var/tmp/a114r4/singleN.out | cut -c1-300
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
echo "== E-new-2(b): site $S, mailboxes, autoresponder on the fresh candidate Arch server $(ts)"
bash $H/mksite.sh $CELL main $S $Z $IP $IP
ver $CELL main
OWN $CELL main autoresp_first $S expect-none
OWN $CELL main autoresp_e4 $S mail.$Z basic
echo "== E-new-5 on the candidate panel: lab 115 label, installer refusal, automatic rollback $(ts)"
[ -d /var/tmp/a114r4/portal-v0.1.0-alpha.115 ] || bash $H/portal.sh $C115 2>&1 | tail -3
bash $H/portalnet.sh add v0.1.0-alpha.115 | tail -4
bash $H/portalnet.sh latest v0.1.0-alpha.115
b1 sh $CELL main "echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env" --label lab-trigger --limit 300 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip())'
bash $H/portalnet.sh trust $CELL main
OWN $CELL main update2 failed | tee /var/tmp/a114r4/e5-114.txt
ver $CELL main
bash $H/gq.sh $CELL main "cat /opt/celikpanel/release.commit; journalctl --no-pager -o cat --since -30min | grep -E 'CELIKPANEL_UPDATE_FAILURE|unsupported key' | grep -v 'level=debug' | cut -c1-500 | tail -6; /usr/libexec/celikpanel/recovery status --json 2>/dev/null | head -c 1200"
python3 -I $H/shots.py $CELL main e5-rolled-back '[{"name":"settings-update","path":"/settings","wait":4000,"actions":[{"eval":"document.querySelectorAll(\"details\").forEach(d=>d.open=true)"},{"wait":1000},{"probe":"[...document.querySelectorAll(\"details,[role=alert],[role=status]\")].map(e=>e.innerText.replace(/\s+/g,\" \").slice(0,700))","label":"update notice text"}]}]' --langs en,tr --widths desktop,phone | tail -c 2500
echo "-- trigger removed, the normal update"
b1 sh $CELL main "sed -i '/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d' /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env" --label lab-trigger-off --limit 300 | python3 -c 'import json,sys; print("   trigger lines left:", json.load(sys.stdin)["stdout"].strip())'
OWN $CELL main update2 succeeded
ver $CELL main
echo "== N done $(ts)"
