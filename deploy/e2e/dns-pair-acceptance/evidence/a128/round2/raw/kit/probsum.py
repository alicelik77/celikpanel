import json, sys
d = json.load(open(sys.argv[1]))
try: b = json.loads(d["body"])
except Exception: print("  problems", d.get("status"), (d.get("body") or "")[:300]); sys.exit()
print("  problems", d.get("status"), "checked_at", b.get("checked_at"), "count", len(b.get("problems") or []), "unknown", json.dumps(b.get("unknown"))[:300])
for p in b.get("problems") or []: print("   -", json.dumps(p, ensure_ascii=False)[:400])
