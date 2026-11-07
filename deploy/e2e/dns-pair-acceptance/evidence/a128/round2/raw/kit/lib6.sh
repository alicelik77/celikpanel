# alpha.126 round helpers (sourced after lib.sh + lib5.sh). Lab only.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}
# asn NODE USER METHOD PATH [BODY]: an API call as another lab account on NODE
asn() { local n=$1 u=$2; shift 2; python3 -I $A/as.py $L $CELL $n $T/cred-$u.json "$@"; }
# mkstatic NODE DOMAIN -> id (static site, owner API, delegated to ns1/ns2 of the pair)
mkstatic() { infra delegate $2 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  admn $1 POST /api/v1/domains/create "{\"domain\":\"$2\",\"project_type\":\"static\",\"ssl_type\":\"none\"}" --label create-$2 --timeout 300 | sbrief 300 >&2
  domain_id $2 $1; }
# delst NODE ID: one deletion-status read (status + typed fields)
delst() { admn $1 GET /api/v1/domains/$2/deletion-status --label delstatus 2>/dev/null | python3 -c 'import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b); print(d.get("status"), json.dumps({k:j.get(k) for k in ("status","stage","reason","step","detail","error_line","message","waiting_for") if j.get(k)}, ensure_ascii=False)[:700])
except Exception: print(d.get("status"), b[:200])'; }
# delwatch NODE ID SECONDS: deletion-status every 5 s, print changes, stop at 204/404
delwatch() { local t0=$(date +%s) last="" s
  while [ $(( $(date +%s) - t0 )) -lt $3 ]; do s=$(delst $1 $2)
    [ "$s" != "$last" ] && echo "   $(ts) +$(( $(date +%s) - t0 ))s $s"; last=$s
    case "$s" in 204*|404*) echo "   DELETION-ENDED after $(( $(date +%s) - t0 ))s"; return 0;; esac; sleep ${EVERY:-5}; done
  echo "   still not ended after $3 s"; return 1; }
digz() { echo "  @$MIP $1 SOA: $(dig1 $MIP $1 SOA) [$(rcode $MIP $1)] | @$BIP: $(dig1 $BIP $1 SOA) [$(rcode $BIP $1)]"; }
