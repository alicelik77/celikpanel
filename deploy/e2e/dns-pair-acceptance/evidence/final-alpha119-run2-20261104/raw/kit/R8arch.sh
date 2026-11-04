# alpha.119 run 2 C (Arch): Python 3.14 Flask deploy; run.sh quick (probes + item 18) for items 9 and 18. Lab only.
export CELL=c119 IP=192.0.3.194 Z=c119.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R8arch-$CELL.log) 2>&1
echo "== R8arch $(ts)"
mkcred custc
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custc.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custc | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custc)
infra delegate flaskc.test ns1.$Z $IP ns2.$Z $IP > /dev/null
FL=$(newdomain custc $SUB flaskc.test letsencrypt); echo "flaskc.test id=$FL"
AS_TIMEOUT=300 as custc POST /api/v1/domains/$FL/ssl/letsencrypt '{"auto_renew":true}' | sbrief 200
adm GET "/api/v1/service/install-preview?step=python" --label pyprev > $T/pyprev.json; sbrief 600 < $T/pyprev.json
CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); n=b.get("notable") or []; print(json.dumps(sorted({(x.get("name") or x.get("package")) for x in n if isinstance(x,dict) and (x.get("name") or x.get("package"))})))' $T/pyprev.json)
R=$(rid); svcstart "{\"service_id\":\"python\",\"request_id\":\"$R\",\"check_installs\":true,\"confirmed_installs\":$CONF}" /api/v1/service/install && svcwait $R 900
adm GET /api/v1/admin/runtime-apps --label rta > $T/rta.json
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print("  sandbox", json.dumps(b.get("sandbox"))); [print("  kind", json.dumps({x:k.get(x) for x in ("kind","offering_id","offered","python","level_gap")})[:500]) for k in b["kinds"]]' $T/rta.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([k for k in b["kinds"] if k["kind"]=="python"][0]["risk_digest"])' $T/rta.json)
adm POST /api/v1/admin/runtime-apps/python/offer "{\"risk_digest\":\"$DG\"}" --label offer-py | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], [(k.get("kind"),k.get("offering_id"),k.get("offered")) for k in b.get("kinds",[])] if isinstance(b,dict) and "kinds" in b else d["body"][:400])'
GQT=60 gq g_pyapp2.sh flaskc_test flask
as custc POST /api/v1/domains/$FL/runtime-apps/create '{"kind":"python","entry":"app:app","server":"wsgi","workers":1,"health_path":"/"}' | sbrief 300
op="dep-$(rid | cut -c1-16)"; as custc POST /api/v1/domains/$FL/runtime-apps/deploy "{\"operation_id\":\"$op\"}" | sbrief 200
for i in $(seq 1 90); do s=$(as custc GET /api/v1/domains/$FL/runtime-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); o=b.get("operation") or {}; print(o.get("id"), o.get("state"), o.get("step"), o.get("reason"), (o.get("message") or "")[:300])'); case "$s" in "$op running"*) sleep 10;; "$op "*) break;; *) sleep 10;; esac; done; echo "  deploy: $s"
gsh1 'curl -sk --resolve flaskc.test:443:127.0.0.1 https://flaskc.test/; python3 --version'; lc https - flaskc.test ""
echo "== run.sh quick (both probes + item 18) $(ts)"
timeout 1200 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c 'cd /root/kit/deploy/e2e/sandbox-escape && ./run.sh quick 2>&1'" | grep -vE "^\s*$" | tail -40
echo "== R8arch done $(ts)"
