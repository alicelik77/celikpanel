# alpha.119 A item 3: offer runtime:program, the Go probe as a real uploaded program (panel-runtime.sh), extra checks. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7prog-$CELL.log) 2>&1
DID=$(cat $T/id-prog119.test); SUBA=$(cat $T/suba)
kinds() { adm GET /api/v1/admin/runtime-apps --label rta > $T/rta.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print("  sandbox", json.dumps(b.get("sandbox")))
for k in b.get("kinds",[]): print("  kind", json.dumps({x:k.get(x) for x in ("kind","offering_id","offered","held","accepted_level","accepted_gaps","level_gap","python")})[:500])' $T/rta.json; }
echo "== R7prog $(ts)"; kinds
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([k for k in b["kinds"] if k["kind"]=="program"][0]["risk_digest"])' $T/rta.json)
echo "-- offer runtime:program"; adm POST /api/v1/admin/runtime-apps/program/offer "{\"risk_digest\":\"$DG\"}" --label offer-prog | sbrief 300; kinds
as cust119a GET /api/v1/domains/$DID/runtime-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  domain view: sandbox", json.dumps(b.get("sandbox")), "certificate", json.dumps(b.get("certificate"))[:150], "kinds", [(k.get("kind"),k.get("offered"),k.get("permission_granted"),k.get("refused_by")) for k in b.get("kinds",[])])'
echo "== panel-runtime.sh (program) on the guest $(ts)"
OSF=$(gsh1 "ls /var/www/celikpanel/subscriptions/*/sites/*/public_html/secret.txt" | head -1); echo "other site file: $OSF"
as cust119a GET /api/v1/me/guide-state > /dev/null
cat $T/cred-cust119a.json.cookie | timeout 1500 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/kit/deploy/e2e/sandbox-escape/prt.sh $DID $OSF program" > $T/prt-prog.txt 2>&1; echo "rc=$?"
cat $T/prt-prog.txt | cut -c1-300
cp $T/prt-prog.txt $LOGS/panel-runtime-program-$CELL.txt
as cust119a GET /api/v1/domains/$DID/runtime-apps > $T/rt.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); a=b.get("app") or {}; s=a.get("status") or {}
print("  app", a.get("id"), a.get("state"), a.get("unit"), a.get("url"), "status", json.dumps({k:s.get(k) for k in ("active_state","serving","health","level","gaps","sandbox_proof","sandbox_generation","entry_check")})[:600])
print("  operation", json.dumps(b.get("operation"))[:400])' $T/rt.json
echo "-- HTTPS through the site"; lc https - prog119.test ""
echo "== suspend subA $(ts)"; adm POST /api/v1/subscriptions/$SUBA/suspension '{"action":"suspend","reason":"lab acceptance: suspension stops runtime apps"}' --label suspend | sbrief 200
sleep 15; gsh1 'systemctl list-units "celikrt*" --all --no-legend --plain | head -3'
echo "== resume subA $(ts)"; adm POST /api/v1/subscriptions/$SUBA/suspension '{"action":"resume"}' --label resume | sbrief 200
sleep 20; gsh1 'systemctl list-units "celikrt*" --all --no-legend --plain | head -3'; lc https - prog119.test ""
echo "-- stop"; as cust119a POST /api/v1/domains/$DID/runtime-apps/control '{"action":"stop"}' | sbrief 300; gsh1 'systemctl list-units "celikrt*" --all --no-legend --plain | head -3'
echo "-- logs (secret must be hidden)"; as cust119a GET "/api/v1/domains/$DID/runtime-apps/logs?lines=40" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); L=b.get("lines") or []; print(" ", d["status"], len(L), "lines; secret visible:", any("lab-probe-secret" in x for x in L)); [print("   ", x[:160]) for x in L[-4:]]'
echo "-- remove"; as cust119a POST /api/v1/domains/$DID/runtime-apps/remove "{\"operation_id\":\"rm-prog-$(rid | cut -c1-12)\",\"remove_files\":false,\"confirm_domain\":\"prog119.test\"}" | sbrief 300
sleep 20; as cust119a GET /api/v1/domains/$DID/runtime-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  after remove app:", json.dumps(b.get("app"))[:120], "op:", json.dumps({k:(b.get("operation") or {}).get(k) for k in ("kind","state","reason")}))'
gsh1 'systemctl list-units "celikrt*" --all --no-legend --plain | head -3; ls /run/celikpanel-sandbox/ 2>&1'
echo "== R7prog done $(ts)"
