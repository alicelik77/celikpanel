# topology A line 3 (customer B): Node.js 24 runtime, n8n offer + install, checks. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L3-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
echo "== L3 start $(ts)"
adm GET /api/v1/runtimes/node/lts --label node-lts > $T/lts.json; sbrief 400 < $T/lts.json
NV=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); v=[r["version"].lstrip("v") for r in b.get("releases",[]) if r["version"].lstrip("v").startswith("24.")]; print(v[0] if v else "")' $T/lts.json)
echo "node version $NV"
R=$(rid); svcstart "{\"version\":\"$NV\",\"request_id\":\"$R\"}" /api/v1/runtimes/node; svcwait $R 900
adm GET /api/v1/admin/service-apps --label sa > $T/sa.json; python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); [print("  app", a.get("id"), a.get("version"), "offered", a.get("offered"), "available", a.get("available"), a.get("reason"), (a.get("detail") or "")[:200], "runtime", a.get("runtime_source")) for a in b.get("apps",[])]' $T/sa.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([a for a in b["apps"] if a["id"]=="n8n"][0]["risk_digest"])' $T/sa.json)
echo "-- offer n8n"; adm POST /api/v1/admin/service-apps/n8n/offer "{\"risk_digest\":\"$DG\"}" --label offer-n8n | sbrief 400
infra delegate n8n118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
NID=$(newdomain cust118b $SUBB n8n118.test letsencrypt); echo "n8n118.test id=$NID"; echo $NID > $T/nid
for i in $(seq 1 20); do c=$(as cust118b GET /api/v1/domains/$NID/service-apps | jget certificate); echo "$c" | grep -qiE 'active|valid|"ok"' && break; sleep 15; done; echo "  certificate: $c"
mkcred n8nowner
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"app":"n8n","owner_email":"owner@n8n118.test","owner_password":c["password"]+"Zz9"},open(sys.argv[2],"w"))' $T/cred-n8nowner.json $T/n8n.json
echo "-- install n8n $(ts)"; as cust118b POST /api/v1/domains/$NID/service-apps/install @$T/n8n.json | sbrief 600; rm -f $T/n8n.json
last=""
for i in $(seq 1 120); do
  s=$(as cust118b GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py)
  [ "$s" != "$last" ] && { echo "  $(ts) $s"; last="$s"; }
  case "$s" in *"install=running"*|*"install=failed"*|*"op=failed"*) break ;; esac
  sleep 15
done
GQT=60 gq gn8n.sh
lc https - n8n118.test ""
lc https - n8n118.test "types/nodes.json"
echo "== L3 done $(ts)"
