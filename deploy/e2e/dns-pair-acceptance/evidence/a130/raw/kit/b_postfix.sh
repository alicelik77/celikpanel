# B lane item 4 (Ubuntu): Postfix from Components; no false "Postfix is not running"; a stopped postfix@- is a problem
# with its time; Start brings it back. usage: b_postfix.sh install|watch|stop|start. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B4postfix-$CELL.log) 2>&1
probs() { admn $N GET /api/v1/metrics/problems --label problems | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"])
print("  problems", d["status"], "checked_at", b.get("checked_at"), "unknown", json.dumps(b.get("unknown")))
for p in b.get("problems") or []: print("   ", json.dumps(p, ensure_ascii=False)[:400])'
  admn $N GET /api/v1/metrics/services --label msvc | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b.get("services") if isinstance(b,dict) else b
for r in rows or []:
    if "postfix" in json.dumps(r).lower(): print("   svc", json.dumps(r, ensure_ascii=False)[:500])'; }
units() { gshn $N 'systemctl is-active postfix postfix@- 2>&1 | tr "\n" " "; systemctl show -p SubState,ActiveState postfix postfix@- | tr "\n" " "'; echo; }
case $1 in
install) echo "== $(ts) install Postfix from Components"; installc $N postfix; units; probs ;;
watch) echo "== $(ts) read"; units; probs ;;
stop) echo "== $(ts) systemctl stop postfix@- (lab: the daemon stops, the wrapper stays)"; gshn $N 'systemctl stop postfix@-'; units
  for i in $(seq 1 18); do sleep 10; r=$(admn $N GET /api/v1/metrics/problems --label problems | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join(p["kind"]+":"+(p.get("name") or p.get("unit") or "") for p in b.get("problems") or []))'); echo "  +$((i*10))s $r"; echo "$r" | grep -qi postfix && break; done; probs ;;
start) echo "== $(ts) Start from the panel"; admn $N POST /api/v1/service/action '{"name":"postfix","action":"start"}' --label pf-start --timeout 120 | sbrief 500; sleep 5; units
  for i in $(seq 1 12); do sleep 10; r=$(admn $N GET /api/v1/metrics/problems --label problems | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join(p["kind"]+":"+(p.get("name") or p.get("unit") or "") for p in b.get("problems") or []))'); echo "  +$((i*10))s [$r]"; echo "$r" | grep -qi postfix || break; done; probs ;;
esac
echo "== b_postfix $1 done $(ts)"
