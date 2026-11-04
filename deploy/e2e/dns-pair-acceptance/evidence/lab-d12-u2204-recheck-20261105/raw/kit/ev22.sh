source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/env.sh
for c in u22 d12; do
  echo "== $c GET /api/v1/setup/operation $(ts)"
  b1 api $c main GET /api/v1/setup/operation --label ev-op --limit 400000 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(d.get("status"), b[:600]); sys.exit()
print(d.get("status")); print(json.dumps(j, indent=1)[:6000])' > $LOGS/setup-operation-$c.json; head -c 3500 $LOGS/setup-operation-$c.json; echo
done
echo "== d12 plan: fields about distribution updates"
python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]) if isinstance(d.get("body"),str) else d
def walk(x,p=""):
  if isinstance(x,dict):
    for k,v in x.items():
      if any(s in k for s in ("update","can_start","blocker","digest","distribution","warning","notice")): print(p+k, json.dumps(v)[:700])
      elif isinstance(v,(dict,list)): walk(v,p+k+".")
  elif isinstance(x,list):
    for i,v in enumerate(x[:3]): walk(v,p+str(i)+".")
walk(b)' $LOGS/plan-d12-main.json
echo "== d12 start reply"; grep -A8 '^start 409' $LOGS/single-d12.log | head -12
