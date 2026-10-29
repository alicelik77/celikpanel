import json,sys
d=json.load(sys.stdin)
b=json.loads(d["body"]) if isinstance(d.get("body"),str) else d
if not isinstance(b, dict): print(b); sys.exit()
print(b.get("status"), b.get("phase"), json.dumps(b.get("error"))[:900])
print(" ".join(f'{s["id"]}={s["status"]}' for s in b.get("steps",[])))
