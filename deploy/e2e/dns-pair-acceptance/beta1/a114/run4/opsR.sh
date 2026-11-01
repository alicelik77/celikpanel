#!/bin/bash
# run 3, the alpha.113 Arch server: E-new-5 (113 -> candidate update refused by the installer, automatic rollback,
# then the normal update), then E-new-2(a) (first autoresponder save on the updated server). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=a6-r; IP=192.0.3.182; Z=ar113.test; S=shopr5.test
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL|SUMMARY-LINE|FINAL-STATUS' | cut -c1-2500; }
ver() { b1 api $1 $2 GET /api/v1/panel/version --label version --limit 4000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", sys.argv[1], b.get("version"), (b.get("commit") or "")[:9], "schema", b.get("schema_version"))' $1/$2; }
exec > >(tee -a $LOGS/opsR.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
if [ "${SKIP_SITE:-0}" != 1 ]; then
echo "== site $S with alice@ and bob@ on the alpha.113 Arch server $(ts)"
bash $H/mksite.sh $CELL main $S $Z $IP $IP
fi
bash $H/portalnet.sh trust $CELL main
ver $CELL main
bash $H/gq.sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; dovecot --version"
bash $H/portalnet.sh latest v0.1.0-alpha.114
echo "== E-new-5 (113 -> candidate): lab trigger, an unknown key in panel.env; the candidate's installer refuses it $(ts)"
b1 sh $CELL main "echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env" --label lab-trigger --limit 300 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip())'
OWN $CELL main update2 failed | tee /var/tmp/a114r4/e5-113.txt
grep -q 'PASS main: update accepted' /var/tmp/a114r4/e5-113.txt || { echo "STOP: the update did not start"; exit 4; }
ver $CELL main
bash $H/gq.sh $CELL main "cat /opt/celikpanel/release.commit; journalctl --no-pager -o cat --since -30min | grep -E 'CELIKPANEL_UPDATE_FAILURE|unsupported key|rolled back|rollback_verified' | grep -v 'level=debug' | cut -c1-400 | tail -8; /usr/libexec/celikpanel/recovery status --json 2>/dev/null | head -c 1500"
echo "-- trigger removed, the normal update"
b1 sh $CELL main "sed -i '/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d' /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env" --label lab-trigger-off --limit 300 | python3 -c 'import json,sys; print("   trigger lines left:", json.load(sys.stdin)["stdout"].strip())'
OWN $CELL main update2 succeeded | tee /var/tmp/a114r4/e5-113b.txt
grep -q 'PASS main: update ends succeeded' /var/tmp/a114r4/e5-113b.txt || { echo "STOP: the normal update did not succeed"; exit 5; }
ver $CELL main
echo "== E-new-2(a): first autoresponder save on the updated Arch server $(ts)"
bash $H/gq.sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; systemctl show dovecot -p ActiveEnterTimestamp"
OWN $CELL main autoresp_first $S
T2=$(b1 sh $CELL main "date +%s" --label now --limit 100 | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].strip())')
echo "-- the next save (and mail: 2 delivered, 1 reply, no bounce) $(ts)"
OWN $CELL main autoresp_e4 $S mail.$Z basic
OWN $CELL main reload_clean $T2
echo "== R done $(ts)"
