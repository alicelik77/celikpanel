# run 2 D5 + check 9: Node.js 24, n8n for an admin-owned customer, suspension, reseller refusal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R2n8n-$CELL.log) 2>&1
SUBN=$(cat $T/subn)
echo "== R2n8n $(ts) subN=$SUBN"
adm GET /api/v1/runtimes/node/lts --label node-lts > $T/lts.json
NV=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); v=[r["version"].lstrip("v") for r in b.get("releases",[]) if r["version"].lstrip("v").startswith("24.")]; print(v[0] if v else "")' $T/lts.json); echo "node $NV"
R=$(rid); svcstart "{\"version\":\"$NV\",\"request_id\":\"$R\"}" /api/v1/runtimes/node && svcwait $R 900
adm GET /api/v1/admin/service-apps --label sa > $T/sa.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([a for a in b["apps"] if a["id"]=="n8n"][0]["risk_digest"])' $T/sa.json)
echo "-- offer n8n"; adm POST /api/v1/admin/service-apps/n8n/offer "{\"risk_digest\":\"$DG\"}" --label offer-n8n | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], [(a["id"],a.get("offered"),a.get("available"),a.get("reason")) for a in b.get("apps",[]) if a["id"]=="n8n"])'
infra delegate n8n118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
NID=$(newdomain cust118n $SUBN n8n118.test letsencrypt); echo "n8n118.test id=$NID"; echo $NID > $T/nid
c=$(as cust118n GET /api/v1/domains/$NID/service-apps | jget certificate); echo "  certificate after create: $c"
if ! echo "$c" | grep -qiE 'true'; then AS_TIMEOUT=300 as cust118n POST /api/v1/domains/$NID/ssl/letsencrypt '{"auto_renew":true}' | sbrief 300; fi
mkcred n8nowner
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"app":"n8n","owner_email":"owner@n8n118.test","owner_password":c["password"]+"Zz9","confirm_replace":True},open(sys.argv[2],"w"))' $T/cred-n8nowner.json $T/n8n.json
echo "-- install n8n $(ts)"; as cust118n POST /api/v1/domains/$NID/service-apps/install @$T/n8n.json | sbrief 500; rm -f $T/n8n.json
last=""
for i in $(seq 1 100); do
  s=$(as cust118n GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py)
  [ "$s" != "$last" ] && { echo "  $(ts) $s"; last="$s"; }
  case "$s" in *"install=running active=True serving=True"*|*"install=failed"*|*"op=failed"*) break ;; esac
  sleep 15
done
GQT=60 gq gn8n.sh
lc https - n8n118.test ""; lc https - n8n118.test "types/nodes.json"
susp() {
  local want=$1 i s
  for i in $(seq 1 40); do
    s=$(adm GET /api/v1/subscriptions/$SUBN/suspension --label sus-get | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); o=b.get("operation") or {}; print(b.get("status"), o.get("state"), json.dumps([(e.get("kind"),e.get("target"),e.get("state")) for e in o.get("effects") or []])[:600])')
    case "$s" in "$want "*) case "$s" in *running*|*pending*|*queued*) ;; *) echo "  $s ($(ts))"; return 0 ;; esac ;; esac
    sleep 6
  done; echo "  still: $s"; }
echo "== suspend subN $(ts)"; adm POST /api/v1/subscriptions/$SUBN/suspension '{"action":"suspend","reason":"lab acceptance: suspension stops service apps"}' --label suspend | sbrief 300
susp suspended; GQT=60 gq gn8n.sh | head -4; lc https - n8n118.test ""
echo "== resume subN $(ts)"; adm POST /api/v1/subscriptions/$SUBN/suspension '{"action":"resume"}' --label resume | sbrief 300
susp active; sleep 20; GQT=60 gq gn8n.sh | head -4; lc https - n8n118.test ""
echo "== reseller refusal $(ts)"
adm POST /api/v1/reseller-plans '{"name":"RN8N","max_customers":0,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["app:n8n"]}}' --label rplan-n8n | sbrief 600
adm POST /api/v1/reseller-plans '{"name":"RALL","max_customers":0,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]}}' --label rplan-all | sbrief 300
echo "== R2n8n done $(ts)"
