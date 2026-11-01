#!/bin/bash
# run 5, E-new-5 on the candidate first server (Debian 13): the lab 115 label of the same candidate on the network
# lab portal, a bad panel.env key -> installer refusal -> automatic rollback -> the record (status, reason, detail),
# then the trigger removed and the normal update to 115-lab through the lab panel's own update API. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5/env.sh
source $H/common.sh
CELL=a7-a; N=main
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL|SUMMARY-LINE|FINAL-STATUS|CHECK-AFTER' | cut -c1-5000; }
ver() { b1 api $1 $2 GET /api/v1/panel/version --label version --limit 4000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", sys.argv[1], b.get("version"), (b.get("commit") or "")[:9], "schema", b.get("schema_version"))' $1/$2; }
exec > >(tee -a $LOGS/e5.log) 2>&1
echo "== E-new-5 on $CELL/$N: lab 115 label, installer refusal, automatic rollback $(ts)"
bash $H/portalnet.sh add v0.1.0-alpha.115 | tail -4
bash $H/portalnet.sh latest v0.1.0-alpha.115
bash $H/portalnet.sh trust $CELL $N
ver $CELL $N
b1 sh $CELL $N "echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env" --label lab-trigger --limit 300 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip())'
OWN $CELL $N update2 rolled_back | tee /var/tmp/a114r5/e5-114.txt
python3 - /var/tmp/a114r5/e5-114.txt <<'PY' | tee /var/tmp/a114r5/e5-detail.txt
import json, re, sys
t = open(sys.argv[1]).read()
m = re.search(r"^FINAL-STATUS (.*)$", t, re.M)
d = json.loads(m.group(1)) if m else {}
summ = d.get("summary") or ""
reason = re.search(r" reason=(.*?) detail=", summ); detail = summ.split(" detail=", 1)[1] if " detail=" in summ else None
print("E5-STATUS", d.get("status"), "phase", d.get("phase"))
print("E5-REASON", reason.group(1) if reason else None)
print("E5-DETAIL", repr(detail))
probs = []
if d.get("phase") != "rolled_back" and "state=rolled_back" not in summ: probs.append("not rolled_back")
if not reason or "installer" not in reason.group(1): probs.append("reason does not name the installer")
if not detail: probs.append("detail empty")
else:
    if any(ord(c) > 126 or ord(c) < 32 for c in detail): probs.append("non-ASCII")
    if len(detail) > 240: probs.append("over 240")
    if re.search(r"https?://|(^|[\s=])/(etc|opt|var|usr|root|home|tmp)/", detail): probs.append("path or URL")
    if re.search(r"[A-Za-z0-9_-]{32,}", detail): probs.append("token-like run")
print("E5-DETAIL-CHECK", "PASS" if not probs else "FAIL " + ", ".join(probs))
PY
ver $CELL $N
bash $H/gq.sh $CELL $N "cat /opt/celikpanel/release.commit; systemctl is-active celikpanel-panel celikpanel-agent; journalctl --no-pager -o cat --since -30min | grep -E 'CELIKPANEL_UPDATE_FAILURE|unsupported key|LAB_ROLLBACK|ERROR / HATA' | grep -v 'level=debug' | cut -c1-500 | tail -6"
python3 -I $H/shots.py $CELL $N e5-rolled-back '[{"name":"settings-update","path":"/settings","wait":4000,"actions":[{"eval":"document.querySelectorAll(\"details\").forEach(d=>d.open=true)"},{"wait":1000},{"probe":"[...document.querySelectorAll(\"details,[role=alert],[role=status]\")].map(e=>e.innerText.replace(/\s+/g,\" \").slice(0,700))","label":"update notice text"}]}]' --langs en,tr --widths desktop,phone | tail -c 3000
echo "-- trigger removed; the normal update to 115-lab $(ts)"
b1 sh $CELL $N "sed -i '/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d' /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env" --label lab-trigger-off --limit 300 | python3 -c 'import json,sys; print("   trigger lines left:", json.load(sys.stdin)["stdout"].strip())'
sleep 30
OWN $CELL $N update2 succeeded | tee /var/tmp/a114r5/e5-115.txt
ver $CELL $N
echo "== E5 done $(ts)"
