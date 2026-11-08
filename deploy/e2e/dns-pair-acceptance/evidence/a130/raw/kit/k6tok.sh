# alpha.129 case 6: a domain made and deleted with a remote API token while its DNS publication is pending (named
# stopped on the joined server). Token values never printed. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C6tok-$CELL.log) 2>&1
PID=$(cat $T/planBasic.id); SUB=$(cat $T/sub-cust1.id)
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
CK=$T/cred-cust1.json
as cust1 GET /api/v1/account/api-tokens > /dev/null
tokbody() { python3 -c 'import json,os,sys; c=json.load(open(sys.argv[1])); b={"name":sys.argv[2],"scope":sys.argv[3],"expires_days":30,"password":c["password"]}
fd=os.open(sys.argv[4],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps(b).encode())' $CK $1 $2 $T/tokreq.json; }
mktok() { tokbody $1 $2; raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --out $T/tokresp.json
  python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1])); t=d.get("token","")
fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,t.encode()); print("   token saved:", bool(t), json.dumps({k:d.get(k) for k in ("code","error")})[:200])' $T/tokresp.json $T/tok-$1.txt; rm -f $T/tokresp.json $T/tokreq.json; }
echo "== 6.0 remote API on, plan Basic with remote_api $(ts)"
adm PUT /api/v1/panel/remote-api '{"enabled":true}' --label rapi-on | sbrief 200
adm GET /api/v1/plans --label plans > $T/plans.json
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("plans") if isinstance(b,dict) else b; p=[x for x in l if x.get("name")=="Basic"][0]; r=p.get("rights") or {}
perms=sorted(set((r.get("permissions") or [])+["remote_api"])); out={"name":"Basic","max_domains":5,"max_databases":5,"max_email_accounts":10,"disk_quota_mb":4096,"bandwidth_quota_mb":0,"rights":{"all_permissions":bool(r.get("all_permissions")),"all_software":bool(r.get("all_software",True)),"permissions":perms,"software":r.get("software") or []},"confirm_narrowing":True}
json.dump(out,open(sys.argv[2],"w"))' $T/plans.json $T/planrights.json
adm PUT /api/v1/plans/$PID @$T/planrights.json --label plan-rapi | sbrief 300
mktok mg manage
echo "== 6.1/6.2 create with the token; named on the joined server is paused (SIGSTOP) 3 s later, after the pair check (lab load) $(ts)"
gshn backup 'systemctl is-active named; kill -CONT $(pidof named) 2>/dev/null; true'
infra delegate tokpend.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
( RAW_TIMEOUT=400 raw POST /api/v1/domains/create --as none --bearer-file $T/tok-mg.txt --json "{\"domain\":\"tokpend.$Z\",\"project_type\":\"static\",\"subscription_id\":$SUB}" --show 500 > $T/tokcreate.out 2>&1 ) &
sleep 3; gshn backup 'kill -STOP $(pidof named); ps -o stat= -p $(pidof named)'
wait; cat $T/tokcreate.out
TD=$(domain_id tokpend.$Z main); echo "   tokpend id $TD"
echo "== 6.3 delete with the token (confirm_domain) $(ts)"
RAW_TIMEOUT=300 raw DELETE "/api/v1/domains/$TD?confirm_domain=tokpend.$Z" --as none --bearer-file $T/tok-mg.txt --show 700
echo "   status as the token:"; raw GET /api/v1/domains/$TD/deletion-status --as none --bearer-file $T/tok-mg.txt --show 500
echo "   second press:"; RAW_TIMEOUT=300 raw DELETE "/api/v1/domains/$TD?confirm_domain=tokpend.$Z" --as none --bearer-file $T/tok-mg.txt --show 700
gshn main "journalctl -u celikpanel-panel --since -3min -o cat --no-pager | grep -ciE tokpend"
echo "== 6.4 named resumed (SIGCONT); the deletion finishes by itself $(ts)"
gshn backup 'kill -CONT $(pidof named); systemctl is-active named'
T0=$(date +%s); last=""
while [ $(( $(date +%s) - T0 )) -lt 420 ]; do
  s=$(raw GET /api/v1/domains/$TD/deletion-status --as none --bearer-file $T/tok-mg.txt --show 300 2>&1 | head -1 | cut -c1-300)
  [ "$s" != "$last" ] && { echo "   +$(( $(date +%s) - T0 ))s $s"; last=$s; }
  echo "$s" | grep -qE '^(404|204|410)' && break
  sleep 15
done
echo "   domains list has tokpend: $(domain_id tokpend.$Z main | wc -c)"
echo "   @$MIP tokpend SOA: $(dig1 $MIP tokpend.$Z SOA) | @$BIP: $(dig1 $BIP tokpend.$Z SOA)"
echo "== k6tok done $(ts)"
