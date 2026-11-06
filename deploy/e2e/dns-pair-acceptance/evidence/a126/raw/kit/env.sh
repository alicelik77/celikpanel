# alpha.126 pre-release lab round (PR #300): shared lab helpers (sourced). Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit
B=/var/tmp/a126r1
C125=ed6086469583f8db5ad7061e69a70a6bb42dcdb2  # v0.1.0-alpha.125 published tag (lab copy built here)
C126=7c8453e98c03f38e7a1074a527f6a2a940934f6c  # release/alpha126 head (PR #300)
C118=${C126OVERRIDE:-$C126}
L=$B/h/deploy/e2e/dns-pair-acceptance/beta1
export BETA1_DIST=${BETA1_DIST:-/var/tmp/cp-pair-accept/dist/$C118-acceptance-license/dist.json}
LOGS=$B/logs
mkdir -p $LOGS
b1() { (cd "$L" && python3 -I beta1.py "$@"); }
lab() { python3 -I "$L/lab.py" "$@"; }
infra() { lab ssh beta1-lab labnet -- "sudo -n tee /root/beta1/infra_guest.py >/dev/null" < "$L/infra_guest.py"; lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/beta1/infra_guest.py $*"; }
st() { python3 -c 'import json,sys; d=json.load(sys.stdin); print(sys.argv[1], d.get("status"), (d.get("body") or "")[:int(sys.argv[2])])' "$1" "${2:-0}"; }
opnow() { b1 api $1 $2 GET /api/v1/setup/operation --label watch --limit 200000 2>/dev/null | python3 $H/opsum.py; }
ts() { date -u +%T; }
wait_status() {
    local cell=$1 node=$2; shift 2
    local last="" out s i
    for i in $(seq 1 ${TRIES:-90}); do
        out=$(opnow $cell $node 2>&1)
        s=$(echo "$out" | head -1 | cut -d' ' -f1)
        [ "$(echo "$out" | sed -n 2p)|$(echo "$out" | head -1 | cut -c1-200)" != "$last" ] && { echo "-- $cell/$node $(ts)"; echo "$out" | head -2; last="$(echo "$out" | sed -n 2p)|$(echo "$out" | head -1 | cut -c1-200)"; }
        for w in "$@"; do [ "$s" = "$w" ] && return 0; done
        sleep ${EVERY:-20}
    done
    echo "-- $cell/$node: bound reached without $*"
    return 1
}
inspection() { b1 api $1 $2 GET /api/v1/setup/peer --label peer-view --limit 200000 2>/dev/null | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]) if (d.get("body") or "").startswith("{") else {}
print(sys.argv[1], "role", b.get("role"), "peer_setup", b.get("peer_setup"), "inspection", json.dumps(b.get("inspection")))' $2; }
# hostrec FILE NAME: every object in an API answer naming NAME (the machine host record check)
hostrec() { python3 -c '
import json,sys
d=json.load(open(sys.argv[1])); b=d.get("body") if isinstance(d,dict) and "body" in d else d
try: b=json.loads(b) if isinstance(b,str) else b
except Exception: pass
n=sys.argv[2]; hits=[]
def walk(x,p):
    if isinstance(x,dict):
        if any(isinstance(v,str) and v.rstrip(".")==n for v in x.values()): hits.append((p,x))
        for k,v in x.items(): walk(v,p+"."+k)
    elif isinstance(x,list):
        for i,v in enumerate(x): walk(v,p+"[%d]"%i)
walk(b,"")
print("  host-record hits for", n, len(hits))
for p,x in hits[:6]: print("   ", p, json.dumps(x)[:400])' "$1" "$2"; }
# sethost CELL NODE FQDN: lab load standing for the owner's server name (hostnamectl + /etc/hosts), before install
sethost() { lab ssh $1 $2 -- "sudo -n sh -c 'hostnamectl set-hostname $3; s=\$(echo $3 | cut -d. -f1); sed -i \"/^127.0.1.1 /d\" /etc/hosts; echo \"127.0.1.1 $3 \$s\" >> /etc/hosts; hostname; hostname -f'" 2>&1 | grep -v "unable to resolve"; }
