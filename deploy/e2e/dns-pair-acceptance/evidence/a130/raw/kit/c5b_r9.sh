# alpha.128 case 5b: database connection limit from the plan (MariaDB, customer cust1) and visitor statistics from the
# real access log of site1. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C5b-$CELL.log) 2>&1
S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id)
q() { grun main g_db.sh my mysql $(printf %s "$1" | base64 -w0); }
echo "== 5b.1 plan Basic: database connections 3 $(ts)"
adm PUT /api/v1/plans/$PID '{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":256,"php.max_children":4,"db.max_user_connections":3},"confirm_narrowing":true}' --label plan-conn | sbrief 400
mkcred custdb
as cust1 POST /api/v1/domains/$C1/databases "{\"name\":\"app\",\"type\":\"mysql\",\"password\":\"$(pw custdb)\"}" | sbrief 400
q "select User, max_user_connections from mysql.user where max_user_connections > 0 or User like '%cust1%';"
echo "   admin's site1 users (no ceiling):"; q "select User, max_user_connections from mysql.user where User like 'site1%';"
echo "   the plan's limit changes to 5 -> existing user follows"
adm PUT /api/v1/plans/$PID '{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":256,"php.max_children":4,"db.max_user_connections":5},"confirm_narrowing":true}' --label plan-conn5 | sbrief 300
sleep 15; q "select User, max_user_connections from mysql.user where max_user_connections > 0;"
echo "== 5b.2 visitor statistics from site1's access log $(ts)"
lab ssh beta1-lab labnet -- "for i in \$(seq 1 12); do curl -s -o /dev/null -A 'Mozilla/5.0 lab-visitor-'\$((i%4)) -H 'Host: site1.$Z' -e http://ref.lab.test/page http://$MIP/home.html; done; for i in 1 2 3; do curl -s -o /dev/null -A 'Googlebot/2.1' -H 'Host: site1.$Z' http://$MIP/; done; for i in 1 2; do curl -s -o /dev/null -A 'Mozilla/5.0 lab' -H 'Host: site1.$Z' http://$MIP/nope-\$i; done; echo sent" 2>/dev/null
adm GET /api/v1/domains/$S1/stats/visitors --label visitors | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], "today", json.dumps(b.get("today")), "read_failed", b.get("read_failed"), "partial", b.get("partial"))
print("   days", len(b.get("days") or []), "top_pages", json.dumps(b.get("top_pages"))[:300], "top_referrers", json.dumps(b.get("top_referrers"))[:200])'
echo "   customer cust1 asks site1's statistics:"; as cust1 GET /api/v1/domains/$S1/stats/visitors | sbrief 150
gshn main "python3 -c 'import sqlite3,glob; f=[x for x in glob.glob(\"/var/lib/celikpanel/*.db\")][0]; c=sqlite3.connect(\"file:\"+f+\"?mode=ro\",uri=True); print([r[1] for r in c.execute(\"pragma table_info(site_visitor_stats_daily)\")]); print(c.execute(\"select count(*) from site_visitor_stats_daily\").fetchone())'"
echo "== 5b.3 server IP addresses (administrator, read only) $(ts)"; adm GET /api/v1/network/ip-addresses --label ips | sbrief 500
echo "== c5b done $(ts)"
