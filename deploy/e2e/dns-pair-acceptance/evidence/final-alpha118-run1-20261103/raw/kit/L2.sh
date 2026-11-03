# topology A line 2 (customer B): app catalog installs and one removal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L2-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
echo "== L2 start $(ts) subB=$SUBB"
for a in nextcloud drupal joomla prestashop mediawiki phpbb; do adm PUT /api/v1/offered-versions "{\"offering_id\":\"app:$a\",\"offered\":true}" --label offer-$a | sbrief 160; done
echo "-- app_installer grant"; adm POST /api/v1/subscriptions/$SUBB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 300
echo "-- customer catalog"; as cust118b GET /api/v1/apps | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], [(a["id"],a.get("version"),a.get("offered")) for a in b.get("apps",[])])'
for a in ${APPS:-nextcloud drupal joomla prestashop mediawiki phpbb}; do
  dn=${PFX:-}${a:0:4}118.test
  DID=$(newdomain cust118b $SUBB $dn); echo "== app $a on $dn (id $DID) $(ts)"; echo "$a $dn $DID" >> $T/apps.txt
  as cust118b GET /api/v1/domains/$DID/apps | python3 -c 'import json,sys; a=sys.argv[1]; b=json.loads(json.load(sys.stdin)["body"]); [print("  req", x.get("id"), json.dumps(x.get("requirements"))[:400], "refused_by", x.get("refused_by")) for x in b.get("apps",[]) if x.get("id")==a]' $a
  t0=$(date +%s); AS_TIMEOUT=1500 as cust118b POST /api/v1/domains/$DID/apps/install "{\"app\":\"$a\"}" | sbrief 900; echo "  took $(( $(date +%s) - t0 ))s"
  as cust118b GET /api/v1/domains/$DID/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  installed", json.dumps({k:x.get(k) for k in ("operation_id","app_id","status","site_url","admin_url","error_kind","last_error","can_remove")})[:700]) for x in b.get("installed",[])]'
  lc get - $dn ""
done
RM=$(grep '^phpbb ' $T/apps.txt | awk '{print $3}'); [ -z "$RM" ] && RM=$(tail -n1 $T/apps.txt | awk '{print $3}')
RMN=$(grep " $RM\$" $T/apps.txt | awk '{print $2}')
OP=$(as cust118b GET /api/v1/domains/$RM/apps | jget installed.0.operation_id)
echo "== remove app on $RMN (op $OP) $(ts)"; AS_TIMEOUT=600 as cust118b DELETE /api/v1/domains/$RM/apps/$OP "{\"confirm\":\"$RMN\",\"remove_database\":true,\"remove_data\":true}" | sbrief 500
as cust118b GET /api/v1/domains/$RM/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  after remove installed:", [(x.get("app_id"),x.get("status")) for x in b.get("installed",[])])'
lc get - $RMN ""
echo "== L2 apps done $(ts)"
