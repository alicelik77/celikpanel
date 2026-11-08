# case 3: site settings within limits (customer cust1 on plan Basic, site cust1.$Z). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C3php-$S.log) 2>&1
D=cust1.$Z
phps() { as cust1 GET /api/v1/domains/$C1/php/settings | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
print(d["status"], "version", b.get("php_version"), "can_change", b.get("can_change"), "pool", b.get("pool_state"), "nginx", b.get("nginx_file_state"))
for l in b.get("limits") or []: print("   ", json.dumps({k:l.get(k) for k in ("field","own","server","ceiling","set_by","effective","source","clamped")}))' ; }
put() { as cust1 PUT /api/v1/domains/$C1/php/settings "{\"memory_mb\":$1,\"time_s\":$2,\"upload_mb\":$3,\"input_vars\":$4,\"processes\":$5,\"display_errors\":\"\",\"error_level\":\"\",\"timezone\":\"\",\"short_open_tag\":\"\"}" | sbrief 700; }
echo "== 3a customer's PHP page $(ts)"; phps
echo "== 3b within the limits: 384 MB, 120 s, 64 MB, 3000 fields, 6 processes $(ts)"; put 384 120 64 3000 6
phps; grun main g_phpt.sh $D pool
echo "== 3c above the server's limit: 1024 MB $(ts)"; put 1024 120 64 3000 6
echo "   600 s run time:"; put 384 600 64 3000 6
echo "   30 processes:"; put 384 120 64 3000 30
echo "== 3d the application cannot raise them (ini_set, set_time_limit, .user.ini) $(ts)"
grun main g_phpt.sh $D put; sleep 6; grun main g_phpt.sh $D ini
echo "   after .user.ini cache (user_ini.cache_ttl 300 s is not waited; .user.ini values read on the first request):"; grun main g_phpt.sh $D ini
echo "== 3e a 140 s script with run time 120 s (stopped) $(ts)"; GQT=500 grun main g_phpt.sh $D sleep 140
grun main g_phpt.sh $D cleanup
echo "== k3php $S done $(ts)"
