# alpha.125 round helpers (sourced after lib.sh). Lab only.
gput() { lab ssh $CELL ${2:-main} -- "sudo -n tee /root/$1 >/dev/null" < $A/$1; }
grun() { local n=$1 f=$2; shift 2; gput $f $n; timeout ${GQT:-400} python3 -I $L/lab.py ssh $CELL $n -- "sudo -n bash /root/$f $*"; }
gshn() { timeout 180 python3 -I $L/lab.py ssh $CELL $1 -- "sudo -n bash -c '$2'"; }
admn() { local n=$1; shift; b1 api $CELL $n "$@" --limit 400000; }
siteof() { grun ${2:-main} g_site.sh $1 2>/dev/null | tail -1; }
cronget() { adm GET /api/v1/domains/$1/cron --label cron-list > $T/cron.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print(b.get("version",""))' $T/cron.json; }
cronadd() { local v; v=$(cronget $1); adm POST /api/v1/domains/$1/cron "{\"schedule\":\"$2\",\"command\":\"$3\",\"version\":\"$v\"}" --label cron-add | sbrief 300; }
ver() { admn ${1:-main} GET /api/v1/panel/version --label version | sbrief 300; }
