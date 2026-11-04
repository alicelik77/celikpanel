import json, sys
d = json.load(sys.stdin)
try:
    b = json.loads(d["body"])
except Exception:
    print("http", d.get("status"), (d.get("body") or "")[:200]); sys.exit()
o = b.get("operation", b)
print(o.get("status"), o.get("backup_name"), o.get("code"), (o.get("detail") or "")[:300])
