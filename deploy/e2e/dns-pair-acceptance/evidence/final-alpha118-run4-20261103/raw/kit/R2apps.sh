# run 2 D3: WordPress refusal, Components PHP 8.4 (distribution), catalog apps on subdomains, Drupal removal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R2apps-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
appview() { as cust118b GET /api/v1/domains/$1/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  installed", json.dumps({k:x.get(k) for k in ("app_id","status","site_url","error_kind","last_error")})[:600]) for x in b.get("installed",[])]'; }
echo "== R2apps $(ts) subB=$SUBB"
for a in nextcloud drupal joomla prestashop mediawiki phpbb; do adm PUT /api/v1/offered-versions "{\"offering_id\":\"app:$a\",\"offered\":true}" --label offer-$a > /dev/null; done
adm POST /api/v1/subscriptions/$SUBB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 200
P=$(newdomain cust118b $SUBB apps118.test); echo "apps118.test id=$P"
W=$(newdomain cust118b $SUBB wp.apps118.test); echo "wp.apps118.test id=$W"
echo "-- WordPress on a server without the extensions (expect refusal with guidance)"
as cust118b GET /api/v1/domains/$W/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  req", x.get("id"), json.dumps(x.get("requirements"))[:700]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
adm GET /api/v1/domains/$W/apps --label admin-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  admin view", x.get("id"), json.dumps(x.get("requirements"))[:700]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
AS_TIMEOUT=600 as cust118b POST /api/v1/domains/$W/apps/install '{"app":"wordpress"}' | sbrief 900
lc code - wp.apps118.test ""
echo "== admin: Components -> PHP-FPM php8.4-fpm (distribution) $(ts)"
R=$(rid); svcstart "{\"service_id\":\"php-fpm\",\"package\":\"php8.4-fpm\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label php-op | sbrief 1200
GQT=60 gq g_php.sh
for a in ${APPS:-nextcloud joomla prestashop mediawiki phpbb drupal}; do
  dn=${a:0:4}.apps118.test
  DID=$(newdomain cust118b $SUBB $dn); echo "== app $a on $dn (id $DID) $(ts)"; echo "$a $dn $DID" >> $T/apps2.txt
  t0=$(date +%s); AS_TIMEOUT=1500 as cust118b POST /api/v1/domains/$DID/apps/install "{\"app\":\"$a\"}" | sbrief 900; echo "  took $(( $(date +%s) - t0 ))s"
  appview $DID; lc code - $dn ""; lc get - $dn ""
done
DD=$(grep '^drupal ' $T/apps2.txt | awk '{print $3}')
OP=$(as cust118b GET /api/v1/domains/$DD/apps | jget installed.0.operation_id)
echo "== remove Drupal (op $OP) $(ts)"; AS_TIMEOUT=600 as cust118b DELETE /api/v1/domains/$DD/apps/$OP '{"confirm":"drup.apps118.test","remove_database":true,"remove_data":true}' | sbrief 600
appview $DD; lc code - drup.apps118.test ""
GQT=60 gq g_drup.sh
echo "== R2apps done $(ts)"
