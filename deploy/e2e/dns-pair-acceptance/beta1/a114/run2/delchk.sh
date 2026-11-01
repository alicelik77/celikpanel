source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
b1 api a4-a backup GET /api/v1/domains/1/deletion-status --label delstatus-jd --limit 3000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("status"), (d.get("body") or "")[:900])'
b1 api a4-a backup GET /api/v1/domains --label domains-backup --limit 20000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("status"), (d.get("body") or "")[:600])'
bash $H/gq.sh a4-a backup "journalctl -u celikpanel-panel -u celikpanel-agent --no-pager -o short-iso --since '08:34:40' | grep -iE 'custb114|delet' | cut -c1-300 | head -20"
