# run 6: document root modes on create, after local restore, and on a WordPress staging copy. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R6-$CELL.log) 2>&1
wpop() { local id=$1 op=$2 s i; for i in $(seq 1 90); do s=$(as cust118a GET /api/v1/wordpress/$id/operations/$op | python3 $A/wpop.py); case "$s" in queued*|backing_up*|updating*|restoring*|cloning*) sleep 10 ;; *) echo "  wp op $op: $s ($(ts))"; return ;; esac; done; echo "  wp op still $s"; }
echo "== R6 $(ts)"
mkcred cust118a
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust118a.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-a | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust118a); DID=$(newdomain cust118a $SUB c118.test); echo "c118.test id=$DID"
echo "== 1 new site"; GQT=60 gq g_mode6.sh c118.test
echo "== 2 backup + restore"
for n in a b; do python3 $A/mkup.py $T/t.json probe-$n.txt text:$n 2>/dev/null; as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 100; done
as cust118a POST "/api/v1/domains/$DID/files?path=/" '{"action":"create","file_name":"sub","is_dir":true}' | sbrief 200
as cust118a POST "/api/v1/domains/$DID/files?path=/sub" "$(python3 -c 'import json,base64; print(json.dumps({"action":"upload","file_name":"inner.txt","content":base64.b64encode(b"inner").decode()}))')" | sbrief 100
GQT=60 gq g_mode6.sh c118.test
as cust118a POST /api/v1/domains/$DID/backups '{"type":"full"}' > $T/bk.json; sbrief 300 < $T/bk.json
BN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("backup") or {}).get("name",""))' $T/bk.json)
AS_TIMEOUT=600 as cust118a POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$BN\"}" | sbrief 400
GQT=60 gq g_mode6.sh c118.test; lc code - c118.test ""; lc code - c118.test "sub/inner.txt"
echo "== 3 WordPress staging copy $(ts)"
R=$(rid); svcstart "{\"service_id\":\"php-fpm\",\"package\":\"php8.4-fpm\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm POST /api/v1/subscriptions/$SUB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 200
WID=$(newdomain cust118a $SUB wp.c118.test); echo "wp.c118.test id=$WID"
AS_TIMEOUT=900 as cust118a POST /api/v1/domains/$WID/apps/install '{"app":"wordpress"}' | sbrief 300
mkcred wpadm; nup labchk.sh >/dev/null; pw wpadm | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh wpfinish $IP wp.c118.test"
SID=$(newdomain cust118a $SUB stage.c118.test); echo "stage.c118.test id=$SID"
as cust118a POST /api/v1/wordpress/$WID/clone "{\"target_domain_id\":$SID}" > $T/wpc.json; sbrief 300 < $T/wpc.json
OP=$(jget operation_id < $T/wpc.json); [ -n "$OP" ] && wpop $WID $OP
GQT=60 gq g_mode6.sh stage.c118.test; GQT=60 gq g_mode6.sh wp.c118.test
lc code - stage.c118.test ""; lc get - stage.c118.test ""
echo "== R6 done $(ts)"
