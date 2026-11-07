# alpha.128 case 2: site PHP limits on the updated main (customer cust1 on plan Basic, site cust1.$Z). Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C2php-$CELL.log) 2>&1
C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id); D=cust1.$Z
phps() { as cust1 GET /api/v1/domains/$C1/php/settings | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
print(d["status"], "version", b.get("php_version"), "can_change", b.get("can_change"), "pool", b.get("pool_state"), "nginx", b.get("nginx_file_state"))
for l in b.get("limits") or []: print("   ", json.dumps({k:l.get(k) for k in ("field","own","server","ceiling","set_by","set_by_name","effective","source","clamped","app_wish_mb")}))' ; }
put() { as cust1 PUT /api/v1/domains/$C1/php/settings "{\"memory_mb\":$1,\"time_s\":$2,\"upload_mb\":$3,\"input_vars\":$4,\"processes\":$5,\"display_errors\":\"\",\"error_level\":\"\",\"timezone\":\"\",\"short_open_tag\":\"\"}" | sbrief 900; }
echo "== server limits (administrator) $(ts)"; adm GET /api/v1/rights/setting-limits --label sl | sbrief 1400
echo "== 2a: customer's PHP page before $(ts)"; phps
grun main g_phpt.sh $D pool
echo "== 2b: values within the limits: 384 MB, 120 s, 64 MB, 3000 fields, 6 processes $(ts)"; put 384 120 64 3000 6
phps; grun main g_phpt.sh $D pool
echo "== 2c: above the server's limit: 1024 MB -> SETTING_ABOVE_LIMIT $(ts)"; put 1024 120 64 3000 6
echo "   30 processes:"; put 384 120 64 3000 30
echo "== 2d: the application cannot raise them (ini_set, set_time_limit, .user.ini) $(ts)"
grun main g_phpt.sh $D put; sleep 2; grun main g_phpt.sh $D ini
echo "== 2e: a 90 s script with run time 120 s (fastcgi_read_timeout) $(ts)"; grun main g_phpt.sh $D sleep 90
echo "== 2f: plan ceilings lowered: preview names the site $(ts)"
BODY='{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":256,"php.max_children":4}}'
adm POST /api/v1/plans/$PID/preview "$BODY" --label plan-preview | sbrief 1500
adm PUT /api/v1/plans/$PID "$BODY" --label plan-put | sbrief 600
B2=$(echo "$BODY" | sed 's/}}$/},"confirm_narrowing":true}/'); adm PUT /api/v1/plans/$PID "$B2" --label plan-put-confirm | sbrief 600
sleep 25; phps; grun main g_phpt.sh $D pool
echo "   customer asks 6 processes / 384 MB now:"; put 256 120 64 3000 6
echo "   notices:"; adm GET /api/v1/rights/notices --label notices | sbrief 1500
echo "== 2g: notify only $(ts)"
B3=$(echo "$BODY" | sed 's/}}$/},"overuse_policy":"notify","notify_percent_disk":60,"confirm_narrowing":true}/'); adm PUT /api/v1/plans/$PID "$B3" --label plan-notify | sbrief 600
adm GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("plans") if isinstance(b,dict) else b; [print("  ", json.dumps({k:p.get(k) for k in ("id","name","overuse_policy","notify_percent_disk","notify_percent_traffic","ceilings")})) for p in l]'
echo "== c2php done $(ts)"
