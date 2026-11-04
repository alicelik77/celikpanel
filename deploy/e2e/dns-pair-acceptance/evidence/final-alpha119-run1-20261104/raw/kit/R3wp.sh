# run 3 item 2: WordPress refused with missing-extension guidance; then Components php8.4-fpm. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R3wp-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
echo "== R3wp $(ts)"
for a in nextcloud drupal joomla prestashop mediawiki phpbb; do adm PUT /api/v1/offered-versions "{\"offering_id\":\"app:$a\",\"offered\":true}" --label offer-$a > /dev/null; done
adm POST /api/v1/subscriptions/$SUBB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 200
P=$(newdomain cust118b $SUBB apps118.test); echo "apps118.test id=$P"
W=$(newdomain cust118b $SUBB wp.apps118.test); echo "wp.apps118.test id=$W"; echo $W > $T/wid
GQT=60 gq g_docroot.sh wp.apps118.test
as cust118b GET /api/v1/domains/$W/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  customer req", x.get("id"), json.dumps(x.get("requirements"))[:800]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
adm GET /api/v1/domains/$W/apps --label admin-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  admin req", x.get("id"), json.dumps(x.get("requirements"))[:800]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
echo "-- install WordPress (expect refusal, nothing changed)"; AS_TIMEOUT=600 as cust118b POST /api/v1/domains/$W/apps/install '{"app":"wordpress"}' | sbrief 900
as cust118b GET /api/v1/domains/$W/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  installed after refusal:", b.get("installed"))'
GQT=60 gq g_docroot.sh wp.apps118.test
lc code - wp.apps118.test ""
echo "== Components -> php-fpm php8.4-fpm $(ts)"
R=$(rid); svcstart "{\"service_id\":\"php-fpm\",\"package\":\"php8.4-fpm\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
GQT=60 gq g_php.sh
echo "== R3wp done $(ts)"
