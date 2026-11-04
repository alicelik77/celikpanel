# wpstat.py FILE [pick]: WordPress status summary, or the first plugin file with an update. Lab only.
import json, sys
d = json.load(open(sys.argv[1]))
try:
    b = json.loads(d["body"])
except Exception:
    print(d.get("status"), (d.get("body") or "")[:300]); sys.exit()
s = b.get("status") or {}
plugins = s.get("plugins") or []
if len(sys.argv) > 2:
    ps = [p for p in plugins if p.get("update_version")]
    print(ps[0]["file"] if ps else ""); sys.exit()
print(d["status"], "detected", s.get("detected"), "core", s.get("core_version"), "core_update", s.get("core_update"),
      "db_linked", b.get("database_linked"), "code", s.get("code"), "checked", s.get("updates_checked"))
for p in plugins:
    print("  plugin", p.get("file"), p.get("version"), "->", p.get("update_version"), "verifiable", p.get("verifiable"))
for o in (b.get("operations") or [])[:4]:
    print("  op", json.dumps({k: o.get(k) for k in ("operation_id", "kind", "status", "backup_name", "code")}))
