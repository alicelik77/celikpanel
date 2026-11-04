# run 3 item 5: Node.js 24, n8n for an admin-owned customer RUNS (login), suspension, reseller refusals. Lab only.
export CELL=u118 IP=192.0.3.191 Z=u118.test STOPAFTERRUN=1
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/B7n8n-$CELL.log) 2>&1
SUBN=$(cat $T/sub)
if [ -z "${SKIPINSTALL:-}" ]; then
echo "== R3n8n $(ts) subN=$SUBN"
adm GET /api/v1/runtimes/node/lts --label node-lts > $T/lts.json
NV=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); v=[r["version"].lstrip("v") for r in b.get("releases",[]) if r["version"].lstrip("v").startswith("24.")]; print(v[0] if v else "")' $T/lts.json); echo "node $NV"
R=$(rid); svcstart "{\"version\":\"$NV\",\"request_id\":\"$R\"}" /api/v1/runtimes/node && svcwait $R 900
adm GET /api/v1/admin/service-apps --label sa > $T/sa.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([a for a in b["apps"] if a["id"]=="n8n"][0]["risk_digest"])' $T/sa.json)
echo "-- offer n8n"; adm POST /api/v1/admin/service-apps/n8n/offer "{\"risk_digest\":\"$DG\"}" --label offer-n8n | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], [(a["id"],a.get("offered"),a.get("available"),a.get("reason")) for a in b.get("apps",[]) if a["id"]=="n8n"])'
infra delegate n8nb118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
NID=$(newdomain custb $SUBN n8nb118.test letsencrypt); echo "n8nb118.test id=$NID"; echo $NID > $T/nid
c=$(as custb GET /api/v1/domains/$NID/service-apps | jget certificate)
echo "$c" | grep -qi true || AS_TIMEOUT=300 as custb POST /api/v1/domains/$NID/ssl/letsencrypt '{"auto_renew":true}' | sbrief 300
mkcred n8nowner
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"app":"n8n","owner_email":"owner@n8nb118.test","owner_password":c["password"]+"Zz9","confirm_replace":True},open(sys.argv[2],"w"))' $T/cred-n8nowner.json $T/n8n.json
echo "-- install n8n $(ts)"; as custb POST /api/v1/domains/$NID/service-apps/install @$T/n8n.json | sbrief 500; rm -f $T/n8n.json
last=""
for i in $(seq 1 100); do
  s=$(as custb GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py)
  [ "$s" != "$last" ] && { echo "  $(ts) $s"; last="$s"; }
  case "$s" in *"install=running active=True serving=True"*|*"install=failed"*|*"op=failed"*) break ;; esac
  sleep 15
done
fi
NID=$(cat $T/nid)
sleep 20; GQT=60 gq gn8n3.sh; GQT=60 gq g_env5.sh
lc https - n8nb118.test ""; lc https - n8nb118.test "types/nodes.json"
echo "-- owner login"; nup labchk.sh >/dev/null; echo "$(pw n8nowner)Zz9" | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh n8nlogin $IP n8nb118.test"
[ -n "${STOPAFTERRUN:-}" ] && { echo "== R3n8n run-check done $(ts)"; exit 0; }
susp() {
  local want=$1 i s
  for i in $(seq 1 40); do
    s=$(adm GET /api/v1/subscriptions/$SUBN/suspension --label sus-get | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); o=b.get("operation") or {}; print(b.get("status"), o.get("state"), json.dumps([(e.get("kind"),e.get("target"),e.get("state")) for e in o.get("effects") or []])[:600])')
    case "$s" in "$want "*) case "$s" in *running*|*pending*|*queued*) ;; *) echo "  $s ($(ts))"; return 0 ;; esac ;; esac
    sleep 6
  done; echo "  still: $s"; }
echo "== suspend subN $(ts)"; adm POST /api/v1/subscriptions/$SUBN/suspension '{"action":"suspend","reason":"lab acceptance: suspension stops service apps"}' --label suspend | sbrief 300
susp suspended; GQT=60 gq gn8n3.sh; GQT=60 gq g_env5.sh | head -6; lc https - n8nb118.test ""
echo "== resume subN $(ts)"; adm POST /api/v1/subscriptions/$SUBN/suspension '{"action":"resume"}' --label resume | sbrief 300
susp active; sleep 30; GQT=60 gq gn8n3.sh; GQT=60 gq g_env5.sh | head -6; lc https - n8nb118.test ""
echo "== reseller refusals $(ts)"
echo "-- reseller plan naming app:n8n"; adm POST /api/v1/reseller-plans '{"name":"RN8N","max_customers":0,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["app:n8n"]}}' --label rplan-n8n | sbrief 600
echo "-- reseller plan with all software"; RP=$(adm POST /api/v1/reseller-plans '{"name":"RALL","max_customers":0,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]}}' --label rplan-all | tee $T/rp.json | jget id); sbrief 300 < $T/rp.json
mkcred res118; mkcred rcust118
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"reseller","rights":{"state":"plan","plan_id":int(sys.argv[2])}}, open(sys.argv[3],"w"))' $T/cred-res118.json $RP $T/u.json
adm POST /api/v1/users @$T/u.json --label reseller-create | sbrief 300
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-rcust118.json $T/u.json
echo "-- reseller creates a customer"; as res118 POST /api/v1/users @$T/u.json | sbrief 300; rm -f $T/u.json
RS=$(sub_of rcust118); echo "reseller customer subscription $RS"
infra delegate rn8nb118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
RD=$(newdomain rcust118 $RS rn8nb118.test letsencrypt); echo "rn8nb118.test id=$RD"
c=$(as rcust118 GET /api/v1/domains/$RD/service-apps | jget certificate); echo "$c" | grep -qi true || AS_TIMEOUT=300 as rcust118 POST /api/v1/domains/$RD/ssl/letsencrypt '{"auto_renew":true}' | sbrief 300
as rcust118 GET /api/v1/domains/$RD/service-apps | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  reseller-customer view:", [(a.get("id"),a.get("offered"),a.get("refused_by"),a.get("reason")) for a in b.get("apps",[])])'
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"app":"n8n","owner_email":"owner@rn8nb118.test","owner_password":c["password"]+"Zz9","confirm_replace":True},open(sys.argv[2],"w"))' $T/cred-rcust118.json $T/rn.json
echo "-- install n8n under the reseller-owned subscription (expect licence refusal)"; as rcust118 POST /api/v1/domains/$RD/service-apps/install @$T/rn.json | sbrief 600
echo "-- same request by the administrator"; adm POST /api/v1/domains/$RD/service-apps/install @$T/rn.json --label admin-install-reseller-sub | sbrief 600; rm -f $T/rn.json
echo "== R3n8n done $(ts)"
