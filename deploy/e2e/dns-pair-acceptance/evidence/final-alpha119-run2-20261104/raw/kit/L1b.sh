source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L1b-$CELL.log) 2>&1
echo "== PHP uploads recheck one more round after the disk limit was lifted $(ts)"
as cust118a GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b["subscriptions"][0]["usage"]["disk"]; print({k:s.get(k) for k in ("state","uploads_blocked","uploads_applied","uploads_note","over_since","measured_at")})'
waitround
as cust118a GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b["subscriptions"][0]["usage"]["disk"]; print({k:s.get(k) for k in ("state","uploads_blocked","uploads_applied","uploads_note","over_since","measured_at")})'
lc phpup - c118.test
cat > $A/g_tmp5.sh <<'G'
grep -rn "file_uploads" /etc/php/*/fpm/pool.d/ 2>/dev/null; ls -la --time-style=+%T /etc/php/8.4/fpm/pool.d/ | grep -E "site2|c118" 
python3 -c 'import sqlite3; db=sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro",uri=True); print([r for r in db.execute("select * from subscription_disk_usage")])' 2>&1 | cut -c1-600
journalctl -u celikpanel-panel --no-pager --since -25min 2>/dev/null | grep -iE "\[disk\]|upload" | tail -8 | cut -c1-300
G
gq g_tmp5.sh
