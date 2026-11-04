# alpha.119 run 2 item 1: Python 3.13: install + offer, Flask from wheels, FastAPI (asgi), broken Flask redeploy
# falls back, wsgi probe full matrix. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R8py-$CELL.log) 2>&1
PY=$(cat $T/id-py119.test); FL=$(cat $T/id-flask119.test); FA=$(cat $T/id-fast119.test)
view() { as cust119b GET /api/v1/domains/$1/runtime-apps > $T/rv.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); a=b.get("app") or {}; s=a.get("status") or {}; o=b.get("operation") or {}
print("  app", a.get("id"), a.get("kind"), a.get("state"), "python", json.dumps(a.get("python"))[:200], "status", json.dumps({k:s.get(k) for k in ("active_state","serving","health","level","gaps","sandbox_generation")}))
print("  op", json.dumps({k:o.get(k) for k in ("id","kind","state","step","class","reason","message")})[:900])' $T/rv.json; }
deploy() { local d=$1 op="dep-$(rid | cut -c1-16)" i s
  as cust119b POST /api/v1/domains/$d/runtime-apps/deploy "{\"operation_id\":\"$op\"}" | sbrief 300
  for i in $(seq 1 90); do s=$(as cust119b GET /api/v1/domains/$d/runtime-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); o=b.get("operation") or {}; print(o.get("id"), o.get("state"), o.get("step"))'); case "$s" in "$op running"*|"$op queued"*) sleep 10;; "$op "*) break;; *) sleep 10;; esac; done
  echo "  deploy $op: $s ($(ts))"; view $d; }
echo "== R8py $(ts)"
adm GET "/api/v1/service/install-preview?step=python" --label pyprev > $T/pyprev.json; sbrief 600 < $T/pyprev.json
CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); n=b.get("notable") or []; print(json.dumps(sorted({(x.get("name") or x.get("package")) for x in n if isinstance(x,dict) and (x.get("name") or x.get("package"))})))' $T/pyprev.json); echo "confirming: $CONF"
R=$(rid); svcstart "{\"service_id\":\"python\",\"request_id\":\"$R\",\"check_installs\":true,\"confirmed_installs\":$CONF}" /api/v1/service/install && svcwait $R 900
adm GET /api/v1/admin/runtime-apps --label rta > $T/rta.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([k for k in b["kinds"] if k["kind"]=="python"][0]["risk_digest"])' $T/rta.json)
echo "-- offer component:python"; adm POST /api/v1/admin/runtime-apps/python/offer "{\"risk_digest\":\"$DG\"}" --label offer-py | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], [(k.get("kind"),k.get("offering_id"),k.get("offered"),(k.get("python") or {}).get("ready")) for k in b.get("kinds",[])] if isinstance(b,dict) and "kinds" in b else d["body"][:400])'
echo "== Flask app from wheels $(ts)"
GQT=60 gq g_pyapp2.sh flask119_test flask
as cust119b POST /api/v1/domains/$FL/runtime-apps/create '{"kind":"python","entry":"app:app","server":"wsgi","workers":1,"health_path":"/"}' | sbrief 300
deploy $FL; lc https - flask119.test ""; gsh1 'curl -sk --resolve flask119.test:443:127.0.0.1 https://flask119.test/'
echo "== FastAPI (asgi) $(ts)"
GQT=60 gq g_pyapp2.sh fast119_test fastapi
as cust119b POST /api/v1/domains/$FA/runtime-apps/create '{"kind":"python","entry":"main:app","server":"asgi","workers":1,"health_path":"/"}' | sbrief 300
deploy $FA; gsh1 'curl -sk --resolve fast119.test:443:127.0.0.1 https://fast119.test/'; lc https - fast119.test ""
echo "== Flask redeploy with a broken module (expect fallback to the previous environment) $(ts)"
GQT=60 gq g_pyapp2.sh flask119_test flask-broken
deploy $FL; gsh1 'curl -sk --resolve flask119.test:443:127.0.0.1 https://flask119.test/; echo; ls -d $(getent passwd flask119_test | cut -d: -f6)/app/venv-*'
echo "== wsgi probe via panel-runtime.sh KIND=python (full matrix) $(ts)"
OSF=$(gsh1 "ls /var/www/celikpanel/subscriptions/*/sites/*/public_html/secret.txt" | head -1)
as cust119b GET /api/v1/me/guide-state > /dev/null
cat $T/cred-cust119b.json.cookie | timeout 1800 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/kit/deploy/e2e/sandbox-escape/prt.sh $PY $OSF python" > $T/prt-py.txt 2>&1; echo "rc=$?"
cut -c1-300 $T/prt-py.txt; cp $T/prt-py.txt $LOGS/panel-runtime-python-$CELL.txt
echo "== R8py done $(ts)"
