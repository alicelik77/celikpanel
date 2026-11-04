# alpha.118 lab run 1: shared lab helpers (sourced). Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit
B=/var/tmp/a118r1
C118=ba0a53d1119bb045bcb650ba78064712a281c840  # v0.1.0-alpha.121 tag (name kept)
C117=bb3aa249d259ce4b84aab44afd4b89456415bf59  # alpha.120 (name kept)
L=$B/h/deploy/e2e/dns-pair-acceptance/beta1
export BETA1_DIST=/var/tmp/cp-pair-accept/dist/$C118-acceptance-license/dist.json
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
