# alpha.114 final lab acceptance run 1: shared lab helpers (sourced). Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5
C114=c3f4b328b6640482ea962a1e90eae79a5996c061
C115=77cee8678725d2b00381f9ba51967d56b6c9a878
C113=d3f3a323da46ce8f8dca5ad4385666c5428c353e
L=/var/tmp/a114r5/h/deploy/e2e/dns-pair-acceptance/beta1
export BETA1_DIST=/var/tmp/cp-pair-accept/dist/$C114-acceptance-license/dist.json
LOGS=/var/tmp/a114r5/logs
mkdir -p $LOGS
b1() { (cd "$L" && python3 -I beta1.py "$@"); }
lab() { python3 -I "$L/lab.py" "$@"; }
infra() { lab ssh beta1-lab labnet -- "sudo -n tee /root/beta1/infra_guest.py >/dev/null" < "$L/infra_guest.py"; lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/beta1/infra_guest.py $*"; }
st() { python3 -c 'import json,sys; d=json.load(sys.stdin); print(sys.argv[1], d.get("status"), (d.get("body") or "")[:int(sys.argv[2])])' "$1" "${2:-0}"; }
opnow() { b1 api $1 $2 GET /api/v1/setup/operation --label watch --limit 200000 2>/dev/null | python3 $H/opsum.py; }
q() { lab ssh "$1" "$2" -- "sudo -n tee /root/q.sh >/dev/null" < "$H/$3"; timeout 600 python3 -I "$L/lab.py" ssh "$1" "$2" -- "sudo -n bash /root/q.sh ${4:-}"; }
ts() { date -u +%T; }
wait_status() {
    local cell=$1 node=$2; shift 2
    local last="" out s i
    for i in $(seq 1 ${TRIES:-90}); do
        out=$(opnow $cell $node 2>&1)
        s=$(echo "$out" | head -1 | cut -d' ' -f1)
        [ "$(echo "$out" | sed -n 2p)|$(echo "$out" | head -1 | cut -c1-200)" != "$last" ] && { echo "-- $cell/$node $(ts)"; echo "$out" | head -4; last="$(echo "$out" | sed -n 2p)|$(echo "$out" | head -1 | cut -c1-200)"; }
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
