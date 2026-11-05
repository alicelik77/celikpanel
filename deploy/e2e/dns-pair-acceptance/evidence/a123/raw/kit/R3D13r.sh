# alpha.123 case 3 retry on d13 (894fd63c9): remove-together sent as the fixed page sends it (header = body request_id);
# re-attach after an immediate page change and after a panel restart; PHP version removal retried. Lab only.
export CELL=d13 IP=192.0.3.192 Z=d13.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh
exec > >(tee -a $LOGS/R3r-$CELL.log) 2>&1
rtpost() { python3 -I $A/rt.py $L $CELL main /api/v1/service/uninstall "$1"; }
echo "== 3.3r remove PostgreSQL together with phpPgAdmin; the tab is closed at once $(ts)"
impact "service_id=postgresql"; TOG=$(together_of)
R=$(rid); rtpost "{\"service_id\":\"postgresql\",\"remove_together\":$TOG,\"request_id\":\"$R\"}" > $T/rt1.json; sbrief 700 < $T/rt1.json
OID=$(jget operation.id < $T/rt1.json)
echo "-- new page (active read)"; opread "active=1"
echo "-- reload (the tab's marker, by request id)"; opread "request_id=$R"
follow $OID; comps postgresql,phppgadmin
echo "== 3.4r again with a panel restart in the middle $(ts)"
for s in postgresql phppgadmin; do installc $s; done; comps postgresql,phppgadmin
impact "service_id=postgresql"; TOG=$(together_of)
R=$(rid); rtpost "{\"service_id\":\"postgresql\",\"remove_together\":$TOG,\"request_id\":\"$R\"}" > $T/rt2.json; sbrief 700 < $T/rt2.json
OID=$(jget operation.id < $T/rt2.json); opread "id=$OID"
echo "-- panel restart (as the owner would: systemctl restart celikpanel-panel) $(ts)"
gsh1 'systemctl restart celikpanel-panel; systemctl is-active celikpanel-panel'
panelup
echo "-- new page after the restart (active read)"; opread "active=1"
echo "-- tab marker after the restart"; opread "request_id=$R"
follow $OID; comps postgresql,phppgadmin
gsh1 'journalctl -u celikpanel-panel --since "-4min" -o cat --no-pager | grep -iE "remove.together|resum|service_remove|interrupted" | tail -8; dpkg -l postgresql phppgadmin 2>/dev/null | grep -E "^(ii|rc)" | cut -c1-60'
echo "== 3.5r PHP 8.4 (site123.test and the PHP tools on it) $(ts)"
for i in 1 2 3 4 5 6; do
  adm GET "/api/v1/service/removal-impact?service_id=php-fpm&package=php8.4-fpm" --label impact-php > $T/imp.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); j=json.loads(d["body"]); print("  impact", d["status"], "blockers", json.dumps((j.get("plan") or {}).get("blockers"))[:500]); s=j.get("simulation") or j.get("removal_simulation") or {}; print("  bound_tools", json.dumps({k:v for k,v in j.items() if "tool" in k.lower()})[:400], json.dumps(s)[:400] if s else "")' $T/imp.json
  out=$(uninst '{"service_id":"php-fpm","package":"php8.4-fpm"}'); echo "$out"
  echo "$out" | grep -q service_operation_busy || break; echo "  busy: wait 20 s and ask again"; sleep 20
done
echo "== R3r done $(ts)"
