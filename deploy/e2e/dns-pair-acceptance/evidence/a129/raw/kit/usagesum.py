import json, sys
d = json.load(sys.stdin); s = d.get("status")
try: b = json.loads(d.get("body") or "")
except Exception: print("  usage", s, (d.get("body") or "")[:300]); sys.exit()
t = b.get("traffic") or {}
print("  usage", s, "traffic:", json.dumps({k: t.get(k) for k in ("state", "last_collected", "unknown", "unknown_sites", "unknown_sites_more", "measured", "domains") if k in t})[:900])
