#!/bin/bash
# run 3 stop evidence (read only): setup operations, agent refusal lines, Dovecot versions, the Arch update record. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
exec > >(tee /var/tmp/a114r4/stop-evidence.out) 2>&1
date -u
for n in main backup; do
  echo "===== a6-a $n setup operation"; opnow a6-a $n | cut -c1-1500
  bash $H/gq.sh a6-a $n "grep PRETTY_NAME /etc/os-release; dovecot --version; cat /opt/celikpanel/release.commit 2>/dev/null; echo '-- first refusal lines'; journalctl -u celikpanel-agent --no-pager -o short-iso | grep -m4 -E 'not pinned|fail-closed' | cut -c1-700; echo '-- counts'; journalctl -u celikpanel-agent --no-pager | grep -c 'was not pinned during preflight'; journalctl -u celikpanel-agent --no-pager | grep -c 'Every change on this server is held'; echo '-- doveadm log errors (SIGHUP lines, read only)'; doveadm log errors 2>&1 | grep -i sighup | tail -2 | cut -c1-200; journalctl -u dovecot --no-pager -o cat | grep -i sighup | tail -1; systemctl is-active dovecot postfix celikpanel-agent celikpanel-panel | paste -sd' '" 60000
done
python3 -I $H/shots.py a6-a main stop-main '[{"name":"setup-stuck","path":"/setup","wait":4000},{"name":"dashboard","path":"/","wait":4000}]' --langs en --widths desktop | tail -c 400
python3 -I $H/shots.py a6-a backup stop-backup '[{"name":"setup-stuck","path":"/setup","wait":4000}]' --langs en --widths desktop | tail -c 400
echo "===== a6-r main (alpha.113 Arch, the normal update after the rollback probe)"
b1 api a6-r main GET /api/v1/panel/version --label version --limit 3000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("status"), (d.get("body") or "")[:300])'
python3 - <<'PY'
import json
rows=[json.loads(l) for l in open("/var/tmp/a114r4/owner-log/a6-r.jsonl") if l.strip()]
for r in [x for x in rows if x.get("label") in ("update start","update status")][-3:]:
    print(r.get("label"), r.get("status"), json.dumps(r.get("answer"))[:900])
PY
bash $H/gq.sh a6-r main "systemctl list-units --all --no-legend 'celikpanel-self-update-*' 'celikpanel-release-recovery*' | cut -c1-160; /usr/libexec/celikpanel/recovery status --json 2>&1 | head -c 1500; echo; journalctl --no-pager -o short-iso --since -40min | grep -E '!!|==> ' | grep -vE 'multiple trigger' | cut -c1-300 | tail -30" 80000
