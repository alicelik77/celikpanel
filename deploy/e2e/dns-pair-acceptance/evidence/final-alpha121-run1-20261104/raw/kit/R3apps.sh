# run 3 item 3: PrestaShop, MediaWiki, phpBB, then Drupal install + remove. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R3apps-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
appview() { as cust118b GET /api/v1/domains/$1/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  installed", json.dumps({k:x.get(k) for k in ("operation_id","app_id","status","site_url","error_kind","last_error")})[:700]) for x in b.get("installed",[])] or print("  installed: []")'; }
for a in ${APPS:-prestashop mediawiki phpbb drupal}; do
  dn=${a:0:4}.apps118.test
  DID=$(newdomain cust118b $SUBB $dn); echo "== app $a on $dn (id $DID) $(ts)"; echo "$a $dn $DID" >> $T/apps3.txt
  t0=$(date +%s); AS_TIMEOUT=1500 as cust118b POST /api/v1/domains/$DID/apps/install "{\"app\":\"$a\"}" | sbrief 900; echo "  took $(( $(date +%s) - t0 ))s"
  appview $DID; lc code - $dn ""; lc get - $dn ""
done
DD=$(grep '^drupal ' $T/apps3.txt | awk '{print $3}')
OP=$(as cust118b GET /api/v1/domains/$DD/apps | jget installed.0.operation_id)
echo "== remove Drupal (op $OP) $(ts)"; AS_TIMEOUT=600 as cust118b DELETE /api/v1/domains/$DD/apps/$OP '{"confirm":"drup.apps118.test","remove_database":true,"remove_data":true}' | sbrief 600
appview $DD; lc code - drup.apps118.test ""; GQT=60 gq g_docroot.sh drup.apps118.test
echo "== R3apps done $(ts)"
