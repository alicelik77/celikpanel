# after a reboot: read the setup operation every second, print each distinct (status, phase, code) until a target phase
cell=$1; node=$2; until=$3; max=${4:-240}
last=""
for i in $(seq 1 $max); do
  out=$($B api $cell $node GET /api/v1/setup/operation --label boot-watch --limit 100000 2>/dev/null | python3 -c 'import json,sys
try:
  b=json.loads(json.load(sys.stdin)["body"]); e=b.get("error") or {}; print(b.get("status"), b.get("phase"), e.get("code"), "busy_since="+str(e.get("busy_since","")) if e.get("busy_since") else "")
except Exception as x: print("unreadable")' 2>/dev/null)
  [ "$out" != "$last" ] && echo "$(date -u +%T) $out" && last="$out"
  case "$out" in *"$until"*) break;; esac
  sleep 1
done
