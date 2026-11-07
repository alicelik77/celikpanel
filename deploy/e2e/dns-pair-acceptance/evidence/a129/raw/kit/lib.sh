# alpha.118 run 1 shared helpers (sourced). CELL/IP/Z from the caller or defaults (topology A). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
source $H/common.sh
CELL=${CELL:-a118}; IP=${IP:-192.0.3.190}; Z=${Z:-f118.test}
T=$B/t-$CELL; mkdir -p $T; chmod 700 $T
mkcred() { [ -f $T/cred-$1.json ] || python3 -c 'import json,os,secrets,sys; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT,0o600); os.write(fd,json.dumps({"username":sys.argv[1],"password":secrets.token_urlsafe(15)+"a1A"}).encode())' $1 $T/cred-$1.json; }
pw() { python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["password"])' $T/cred-$1.json; }
as() { local u=$1; shift; python3 -I $A/as.py $L $CELL main $T/cred-$u.json "$@"; }
adm() { b1 api $CELL main "$@" --limit 400000; }
rid() { python3 -c 'import secrets; print(secrets.token_hex(16))'; }
# sbrief: status + body with every password/secret-looking key dropped
sbrief() { python3 -c '
import json,re,sys
d=json.load(sys.stdin); s=d.get("status"); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(s, b[:int(sys.argv[1])]); sys.exit()
def clean(x):
    if isinstance(x,dict): return {k:("[REDACTED]" if re.search("pass|secret|private_key|token",k,re.I) and v else clean(v)) for k,v in x.items()}
    if isinstance(x,list): return [clean(v) for v in x]
    return x
print(s, json.dumps(clean(j))[:int(sys.argv[1])])' ${1:-1200}; }
jget() { python3 -c '
import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(""); sys.exit()
cur=j
for p in sys.argv[1].split("."):
    if isinstance(cur,list): cur=cur[int(p)] if p.isdigit() and int(p)<len(cur) else None
    elif isinstance(cur,dict): cur=cur.get(p)
    else: cur=None
print(json.dumps(cur) if isinstance(cur,(dict,list)) else ("" if cur is None else cur))' "$1"; }
sub_of() { adm GET /api/v1/subscriptions --label subs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print([s["id"] for s in b["subscriptions"] if s.get("owner")==sys.argv[1]][0])' $1; }
did_of() { domain_id $1; }
# guest read-only observation script: gq FILE [ARGS]
gq() { local f=$1; shift; lab ssh $CELL main -- "sudo -n tee /root/$f >/dev/null" < $A/$f; timeout ${GQT:-400} python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/$f $*"; }
gsh1() { timeout 120 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c '$1'"; }
nup() { lab ssh beta1-lab labnet -- "sudo -n tee /root/$1 >/dev/null" < $A/$1; }
nrun() { timeout ${NRT:-400} python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/$*"; }
lc() { local mode=$1 cred=$2 who=$3 hn=${4:-}; nup labchk.sh >/dev/null; if [ "$cred" = - ]; then echo x; else pw $cred; fi | timeout 180 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh $mode $IP $who $hn"; }
site() { lc site - $1; }
newdomain() { # newdomain USER SUB NAME [ssl]  -> prints id
  as $1 POST /api/v1/domains/create "{\"domain\":\"$3\",\"project_type\":\"php\",\"ssl_type\":\"${4:-none}\",\"subscription_id\":$2}" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: j={}
print(j.get("DomainID") or ("ERR %s %s" % (d.get("status"), b[:300])))'; }
lastcol() { adm GET /api/v1/usage/status --label usage-status | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print((b.get("traffic") or {}).get("last_collected"))'; }
# waitround: until the panel's 5-minute usage round has run once more
waitround() { local a=$(lastcol) b i; for i in $(seq 1 45); do sleep 15; b=$(lastcol); [ "$b" != "$a" ] && { echo "  usage round ran: $a -> $b ($(ts))"; sleep 20; return 0; }; done; echo "  NO usage round within 11 min (last $a)"; return 1; }
subusage() { as $1 GET /api/v1/my/subscription | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"])
def find(x):
    if isinstance(x,dict):
        if "traffic_used_bytes" in x or "disk_used_bytes" in x: return x
        for v in x.values():
            r=find(v)
            if r: return r
    if isinstance(x,list):
        for v in x:
            r=find(v)
            if r: return r
u=find(b) or {}
keys=("traffic_used_bytes","traffic_limit_bytes","traffic_limited","traffic_measure","disk_used_bytes","disk_limit_bytes","disk")
print(d["status"], json.dumps({k:u.get(k) for k in keys})[:900])'; }
setq() { as_admin_rights $1 "$2"; }
as_admin_rights() { adm PATCH /api/v1/subscriptions/$1/rights "{\"quantities\":$2,\"confirm\":true,\"accept_over_usage\":true}" --label rights-$1 | sbrief 500; }
svcwait() { local rid=$1 n=$(( ${2:-600} / 10 )) s i
  for i in $(seq 1 $n); do s=$(adm GET "/api/v1/service/operation?request_id=$rid" --label svcop | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); op=b.get("operation",b); print(op.get("status"), op.get("phase"), json.dumps(op.get("error") or op.get("error_code") or "")[:300])' 2>&1)
    case "$s" in succeeded*|failed*) echo "  op $rid: $s ($(ts))"; return 0;; esac; sleep 10; done; echo "  op $rid still: $s"; return 1; }
svcstart() { local i r; for i in $(seq 1 40); do r=$(adm POST $2 "$1" --label svc-start | sbrief 600); echo "$r" | grep -qiE '^409.*(busy|another|in progress)' || { echo "$r"; return 0; }; sleep 15; done; echo "$r"; }
dbt() { lab ssh $CELL main -- "sudo -n tee /root/guestdb.sh >/dev/null" < $A/guestdb.sh; pw $1 | timeout 60 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/guestdb.sh $2"; }
