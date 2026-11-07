# alpha.128 case 6: remote API tokens on the updated main, customer cust1 (plan Basic). Token values never printed. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C6api-$CELL.log) 2>&1
S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id)
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
CK=$T/cred-cust1.json
as cust1 GET /api/v1/account/api-tokens > /dev/null   # fresh session cookie for cust1
tokbody() { python3 -c 'import json,os,sys; c=json.load(open(sys.argv[1])); b={"name":sys.argv[2],"scope":sys.argv[3],"expires_days":30}
if sys.argv[4]=="pw": b["password"]=c["password"]
fd=os.open(sys.argv[5],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps(b).encode())' $CK $1 $2 $3 $T/tokreq.json; }
mktok() { tokbody $1 $2 pw; raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --out $T/tokresp.json
  python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1])); t=d.get("token","")
fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,t.encode()); print("   token saved:", bool(t), "item", json.dumps(d.get("item") or {k:d.get(k) for k in ("code","error")})[:300])' $T/tokresp.json $T/tok-$1.txt; rm -f $T/tokresp.json; }
echo "== 6.0 the server switch is off by default $(ts)"; adm GET /api/v1/panel/remote-api --label rapi | sbrief 300
as cust1 GET /api/v1/account/api-tokens | sbrief 500
echo "   create while off:"; tokbody off read pw; raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --show 400
echo "   a made-up token while off:"; printf 'cpk_abcdefghijkl_%s' "$(python3 -c 'import secrets;print(secrets.token_urlsafe(32))')" > $T/tok-fake.txt; raw GET /api/v1/domains --as none --bearer-file $T/tok-fake.txt --show 300
echo "== 6.1 switch on; plan permission $(ts)"
adm PUT /api/v1/panel/remote-api '{"enabled":true}' --label rapi-on | sbrief 300
adm GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("plans") if isinstance(b,dict) else b; [print("   plan", p.get("id"), p.get("name"), json.dumps(p.get("rights"))[:600]) for p in l if p.get("name")=="Basic"]'
as cust1 GET /api/v1/account/api-tokens | sbrief 400
echo "== 6.2 step-up: no request id / no password $(ts)"
tokbody nopw read nopw; raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --show 300
raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --show 300
echo "== 6.3 create a read token and a manage token $(ts)"; mktok rd read; mktok mg manage
if [ ! -s $T/tok-mg.txt ]; then
  echo "   plan Basic gets remote_api (its listed permissions + remote_api)"
  adm GET /api/v1/plans --label plans > $T/plans.json
  python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("plans") if isinstance(b,dict) else b; p=[x for x in l if x.get("name")=="Basic"][0]; r=p.get("rights") or {}
perms=sorted(set((r.get("permissions") or [])+["remote_api"])); out={"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0,"rights":{"all_permissions":bool(r.get("all_permissions")),"all_software":bool(r.get("all_software",True)),"permissions":perms,"software":r.get("software") or []},"confirm_narrowing":True}
json.dump(out,open(sys.argv[2],"w"))' $T/plans.json $T/planrights.json
  adm PUT /api/v1/plans/$PID @$T/planrights.json --label plan-rapi | sbrief 600
  mktok rd read; mktok mg manage
fi
as cust1 GET /api/v1/account/api-tokens | sbrief 900
echo "== 6.4 read scope $(ts)"
raw GET /api/v1/domains --as none --bearer-file $T/tok-rd.txt --show 400
raw GET /api/v1/domains/$C1/dns/records --as none --bearer-file $T/tok-rd.txt --show 200
echo "   read token adds a record:"; raw POST /api/v1/domains/$C1/dns/records --as none --bearer-file $T/tok-rd.txt --json '{"name":"tokrd","type":"TXT","content":"\"a128\"","ttl":3600}' --show 300
echo "   another account's domain (admin site1):"; raw GET /api/v1/domains/$S1/dns/records --as none --bearer-file $T/tok-rd.txt --show 300
echo "== 6.5 manage scope $(ts)"
raw POST /api/v1/domains/$C1/dns/records --as none --bearer-file $T/tok-mg.txt --json '{"name":"tokmg","type":"TXT","content":"\"a128 manage\"","ttl":3600}' --show 300
echo "   not-listed routes:"; raw GET /api/v1/domains/$C1/php/settings --as none --bearer-file $T/tok-mg.txt --show 300
raw GET /api/v1/account/api-tokens --as none --bearer-file $T/tok-mg.txt --show 300
raw GET /api/v1/panel/version --as none --bearer-file $T/tok-mg.txt --show 300
echo "   cookie + token:"; raw GET /api/v1/domains --as $CK --bearer-file $T/tok-mg.txt --show 300
echo "== 6.6 a non-cpk Authorization header from a proxy does not break sign-in $(ts)"
python3 -c 'import json,os,sys; c=json.load(open(sys.argv[1])); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"username":c["username"],"password":c["password"]}).encode())' $CK $T/login.json
raw POST /api/v1/auth/login --as none --authz "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJsYWIifQ.c2ln" --json-file $T/login.json --show 200
raw POST /api/v1/auth/login --as none --authz "Basic bGFiOmxhYg==" --json-file $T/login.json --show 200; rm -f $T/login.json
raw GET /api/v1/account/api-tokens --as $CK --authz "Bearer eyJhbGciOiJSUzI1NiJ9.e30.c2ln" --show 200
echo "== 6.7 delete a domain needs confirm_domain $(ts)"
infra delegate tokdel.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
SUB=$(cat $T/sub-cust1.id)
RAW_TIMEOUT=300 raw POST /api/v1/domains/create --as none --bearer-file $T/tok-mg.txt --json "{\"domain\":\"tokdel.$Z\",\"project_type\":\"static\",\"subscription_id\":$SUB}" --show 300
TD=$(domain_id tokdel.$Z main); echo "   tokdel id $TD"
raw DELETE /api/v1/domains/$TD --as none --bearer-file $T/tok-mg.txt --show 300
raw DELETE "/api/v1/domains/$TD?confirm_domain=other.$Z" --as none --bearer-file $T/tok-mg.txt --show 300
RAW_TIMEOUT=600 raw DELETE "/api/v1/domains/$TD?confirm_domain=tokdel.$Z" --as none --bearer-file $T/tok-mg.txt --show 300
echo "== 6.8 switch off: every token 401; on again $(ts)"
adm PUT /api/v1/panel/remote-api '{"enabled":false}' --label rapi-off | sbrief 200
raw GET /api/v1/domains --as none --bearer-file $T/tok-mg.txt --show 200; raw GET /api/v1/domains --as none --bearer-file $T/tok-rd.txt --show 200
adm PUT /api/v1/panel/remote-api '{"enabled":true}' --label rapi-on2 | sbrief 200
raw GET /api/v1/domains --as none --bearer-file $T/tok-rd.txt --show 120
echo "== 6.9 audit rows $(ts)"; adm GET "/api/v1/audit-logs?limit=15" --label audit | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], (d.get("body") or "")[:1200])' | grep -oE '.{0,80}(cpk_|api_token|token)[^,]{0,80}' | head -8
echo "== c6api done $(ts)"
