# run 5 items 3-5: local backup + restore (ownership), WordPress staging copy, MediaWiki, mailbox 90% warning. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R5bk-$CELL.log) 2>&1
SUBA=$(cat $T/suba); DID=$(cat $T/did); SUBB=$(cat $T/subb)
wpop() { local id=$1 op=$2 s i; for i in $(seq 1 90); do s=$(as cust118b GET /api/v1/wordpress/$id/operations/$op | python3 $A/wpop.py); case "$s" in queued*|backing_up*|updating*|restoring*|cloning*) sleep 10 ;; *) echo "  wp op $op: $s ($(ts))"; return ;; esac; done; echo "  wp op still $s"; }
echo "== item 3 backup/restore $(ts)"
python3 $A/mkup.py $T/t.json restore-probe.txt text:before-backup; as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 200
GQT=60 gq g_own5.sh c118.test
as cust118a POST /api/v1/domains/$DID/backups '{"type":"full"}' > $T/bk.json; sbrief 400 < $T/bk.json
BN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("backup") or {}).get("name",""))' $T/bk.json)
python3 $A/mkup.py $T/t.json restore-probe.txt text:after-backup; as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 200
python3 $A/mkup.py $T/t.json added-after.txt text:x; as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 200
echo "-- restore $BN"; AS_TIMEOUT=600 as cust118a POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$BN\"}" | sbrief 600
GQT=60 gq g_own5.sh c118.test
lc site - c118.test
echo "== item 3 WordPress staging copy $(ts)"
R=$(rid); svcstart "{\"service_id\":\"php-fpm\",\"package\":\"php8.4-fpm\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
for a in mediawiki; do adm PUT /api/v1/offered-versions "{\"offering_id\":\"app:$a\",\"offered\":true}" --label offer-$a > /dev/null; done
adm POST /api/v1/subscriptions/$SUBB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 200
WID=$(newdomain cust118b $SUBB wp118.test); echo "wp118.test id=$WID"
AS_TIMEOUT=900 as cust118b POST /api/v1/domains/$WID/apps/install '{"app":"wordpress"}' | sbrief 400
mkcred wpadm; nup labchk.sh >/dev/null; pw wpadm | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh wpfinish $IP wp118.test"
SID=$(newdomain cust118b $SUBB stage.wp118.test); echo "stage.wp118.test id=$SID"
as cust118b POST /api/v1/wordpress/$WID/clone "{\"target_domain_id\":$SID}" > $T/wpc.json; sbrief 400 < $T/wpc.json
OP=$(jget operation_id < $T/wpc.json); [ -n "$OP" ] && wpop $WID $OP
lc get - stage.wp118.test ""; GQT=60 gq g_own5.sh stage.wp118.test
echo "== item 4 MediaWiki $(ts)"
MID=$(newdomain cust118b $SUBB wiki118.test)
AS_TIMEOUT=900 as cust118b POST /api/v1/domains/$MID/apps/install '{"app":"mediawiki"}' | sbrief 500
lc code - wiki118.test "index.php?title=Main_Page"
echo "== item 5 mailbox 90% warning $(ts)"
lab ssh $CELL main -- "sudo -n systemctl start celikpanel-quota-warning.service; sleep 2; sudo -n systemctl is-active celikpanel-quota-warning.service"
GQT=60 gq gW.sh q@c118.test
GQT=60 gq g_marker5.sh
echo "-- second run (must not warn again)"; lab ssh $CELL main -- "sudo -n systemctl start celikpanel-quota-warning.service"; sleep 3
GQT=60 gq gW.sh q@c118.test | grep -E "subject|date.received"
echo "== R5bk done $(ts)"
