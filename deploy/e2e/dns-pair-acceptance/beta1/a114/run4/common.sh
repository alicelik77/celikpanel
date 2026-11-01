# shared helpers for the final alpha.113 ops (sourced after env.sh). Lab only.
brief() { python3 -c '
import json,sys
d=json.load(sys.stdin); s=d.get("status"); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(s, b[:400]); sys.exit()
if isinstance(j, dict):
    keep={k:j.get(k) for k in ("id","code","reason","detail","vars","success","status","state","message","error","domain","proof","deletion","address","suggestion","webmail_url","webmail_ready","project_type","php_version","ssl_enabled","certificate","issuer","expires_at","accounts") if k in j}
    print(s, json.dumps(keep)[:1800] if keep else json.dumps(j)[:1800])
else: print(s, json.dumps(j)[:1200])'; }
dig1() { lab ssh beta1-lab labnet -- "dig +time=2 +tries=1 +norecurse +short @$1 $2 $3" 2>/dev/null | tr '\n' ' ' | sed 's/ *$//'; }
rcode() { lab ssh beta1-lab labnet -- "dig +time=2 +tries=1 +norecurse @$1 $2 SOA" 2>/dev/null | grep -oE 'status: [A-Z]+|flags: [a-z ]+' | tr '\n' ' '; }
poll_dig() {
    local ip=$1 name=$2 type=$3 want=$4 secs=$5 got start=$(date +%s)
    while :; do
        got=$(dig1 $ip $name $type)
        if [ "$want" = EMPTY ]; then [ -z "$got" ] && break; else echo "$got" | grep -Eq -- "$want" && break; fi
        [ $(( $(date +%s) - start )) -ge $secs ] && { echo "   FAIL $ip $name $type: NOT within ${secs}s (got '$got')"; return 1; }
        sleep 3
    done
    echo "   ok $ip $name $type = '${got}' after $(( $(date +%s) - start ))s"
}
domain_id() { b1 api $CELL ${2:-main} GET /api/v1/domains --label list --limit 400000 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"])
def walk(x):
    if isinstance(x,dict):
        if (x.get("name")==sys.argv[1] or x.get("domain_name")==sys.argv[1] or x.get("domain")==sys.argv[1]) and isinstance(x.get("id"),int): print(x["id"]); sys.exit()
        for v in x.values(): walk(v)
    if isinstance(x,list):
        for v in x: walk(v)
walk(b)' $1; }
record_id() { b1 api $CELL ${4:-main} GET /api/v1/domains/$1/dns/records --label records --limit 400000 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b.get("records",b) if isinstance(b,dict) else b
for r in rows:
    if r.get("type")==sys.argv[2] and r.get("name","").rstrip(".")==sys.argv[1]: print(r["id"]); break' $2 $3; }
labq() { local f=$1; shift; lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < "$H/$f"; lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $*"; }
gsh() { b1 sh $CELL $1 "$2" --label "$3" --limit ${4:-6000} | python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("stdout") or d.get("output") or json.dumps(d)).rstrip()[-int(sys.argv[1]):])' ${4:-6000}; }
