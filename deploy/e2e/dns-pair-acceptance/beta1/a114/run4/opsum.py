import json, sys
d = json.load(sys.stdin)
b = json.loads(d["body"]) if isinstance(d.get("body"), str) and d["body"].startswith("{") else d
if not isinstance(b, dict):
    print(b); sys.exit()
op = b.get("operation", b)
print(op.get("status"), op.get("phase"), json.dumps(op.get("error"))[:1500])
print(" ".join(f'{s.get("id")}={s.get("status")}' for s in op.get("steps", [])))
for s in op.get("steps", []):
    if s.get("status") not in ("succeeded", "pending", "completed"):
        print(json.dumps(s)[:1500])
