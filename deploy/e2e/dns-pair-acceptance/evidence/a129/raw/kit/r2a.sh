# alpha.128 round 2, fix A1: a lowered plan keeps the site's own PHP values, a raised plan gives them back. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2a-$CELL.log) 2>&1
C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id); D=cust1.$Z
phps() { as cust1 GET /api/v1/domains/$C1/php/settings | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
for l in b.get("limits") or []:
    if l.get("field") in ("memory","processes"): print("   ", json.dumps({k:l.get(k) for k in ("field","own","ceiling","set_by","effective","clamped")}))' ; }
put() { as cust1 PUT /api/v1/domains/$C1/php/settings "{\"memory_mb\":$1,\"time_s\":120,\"upload_mb\":64,\"input_vars\":3000,\"processes\":$2,\"display_errors\":\"\",\"error_level\":\"\",\"timezone\":\"\",\"short_open_tag\":\"\"}" | sbrief 400; }
plan() { adm PUT /api/v1/plans/$PID "{\"name\":\"Basic\",\"max_domains\":5,\"max_databases\":3,\"max_email_accounts\":10,\"disk_quota_mb\":2048,\"bandwidth_quota_mb\":0,\"ceilings\":$1,\"confirm_narrowing\":true}" --label plan | sbrief 200; }
echo "== A1.0 own values 384 MB / 6 processes $(ts)"; put 384 6; phps
echo "== A1.1 plan lowered to 256 MB / 4 $(ts)"; plan '{"php.memory_limit_mb":256,"php.max_children":4}'; sleep 20; phps
grun main g_phpt.sh $D pool; grun main g_phpt.sh $D put >/dev/null; sleep 2; grun main g_phpt.sh $D ini
echo "   the form sent back unchanged (384/6) is accepted:"; put 384 6
echo "   a new value above the limit (400) is refused:"; put 400 6
echo "== A1.2 plan raised to 512 MB / 20 $(ts)"; plan '{"php.memory_limit_mb":512,"php.max_children":20}'; sleep 20; phps
grun main g_phpt.sh $D pool; grun main g_phpt.sh $D ini
echo "== c-r2a done $(ts)"
