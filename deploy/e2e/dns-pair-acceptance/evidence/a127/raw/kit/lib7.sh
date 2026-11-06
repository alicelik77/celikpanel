# alpha.127 round helpers (sourced after lib.sh + lib5.sh + lib6.sh). Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}
# svcwaitn NODE RID [SECONDS]: a component operation to its end
svcwaitn() { local n=$1 rid=$2 k=$(( ${3:-900} / 10 )) s i
  for i in $(seq 1 $k); do s=$(admn $n GET "/api/v1/service/operation?request_id=$rid" --label svcop 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); op=b.get("operation",b); print(op.get("status"), op.get("phase"), json.dumps(op.get("error") or op.get("error_code") or "")[:400])' 2>&1)
    case "$s" in succeeded*|failed*) echo "  op $rid: $s ($(ts))"; return 0;; esac; sleep 10; done; echo "  op $rid still: $s"; return 1; }
svcstartn() { local n=$1 i r; for i in $(seq 1 40); do r=$(admn $n POST $3 "$2" --label svc-start | sbrief 600); echo "$r" | grep -qiE '^409.*(busy|another|in progress)' || { echo "$r"; return 0; }; sleep 15; done; echo "$r"; }
# installn NODE SERVICE [PACKAGE]
installn() { local R=$(rid) pk=""; [ -n "${3:-}" ] && pk=",\"package\":\"$3\""
  svcstartn $1 "{\"service_id\":\"$2\",\"request_id\":\"$R\"$pk}" /api/v1/service/install; svcwaitn $1 $R ${4:-1200}; }
# mksite NODE DOMAIN TYPE -> id (owner API, delegated to the pair)
mksite() { infra delegate $2 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  admn $1 POST /api/v1/domains/create "{\"domain\":\"$2\",\"project_type\":\"${3:-php}\",\"ssl_type\":\"none\"}" --label create-$2 --timeout 300 | sbrief 300 >&2
  domain_id $2 $1; }
pgrow() { admn ${1:-backup} GET /api/v1/managed-services --label ms | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id") in ("postgresql","phppgadmin"):
        print("  row", json.dumps({k:r.get(k) for k in ("id","is_installed","status","version","package","installed_package","cluster","clusters","notice","kept")})[:900])'; }
# alpha.127: the shared password rule needs a special character
mkcred() { [ -f $T/cred-$1.json ] || python3 -c 'import json,os,secrets,sys; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT,0o600); os.write(fd,json.dumps({"username":sys.argv[1],"password":secrets.token_urlsafe(15)+"-a1A"}).encode())' $1 $T/cred-$1.json; }
# installc NODE SERVICE [PACKAGE] [PHPVER]: install as the Components window does (preview's packages confirmed)
installc() { local n=$1 s=$2 pk=${3:-} pv=${4:-} q R CONF extra=""
  q="step=$s"; [ -n "$pk" ] && q="step=$s:$pk"
  admn $n GET "/api/v1/service/install-preview?$q" --label prev-$s --timeout 150 > $T/prev-$s.json; sbrief 500 < $T/prev-$s.json
  CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps(b.get("installs") or []))' $T/prev-$s.json 2>/dev/null || echo '[]')
  [ -n "$pk" ] && extra=",\"package\":\"$pk\""; [ -n "$pv" ] && extra="$extra,\"php_version\":\"$pv\""
  R=$(rid); svcstartn $n "{\"service_id\":\"$s\"$extra,\"check_installs\":true,\"confirmed_installs\":$CONF,\"request_id\":\"$R\"}" /api/v1/service/install; svcwaitn $n $R 1500; }
