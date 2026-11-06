# softcount.py FILE LABEL: software counts in a rights catalog or a plans answer (lab only)
import json, sys
d = json.load(open(sys.argv[1])); s = d.get("status"); b = d.get("body") or ""
try: j = json.loads(b)
except Exception: print(sys.argv[2], s, b[:300]); sys.exit()
plans = j.get("plans") if isinstance(j, dict) else None
if isinstance(plans, list):
    for p in plans:
        r = p.get("rights") or {}
        sw = r.get("software") or []
        print(" ", sys.argv[2], s, "plan", p.get("id"), p.get("name"), "all_software", r.get("all_software"), "software", len(sw), sorted(sw))
    sys.exit()
out = []
def walk(x, path):
    if isinstance(x, dict):
        for k, v in x.items():
            if isinstance(v, list) and v and all(isinstance(e, (str, dict)) for e in v) and k in ("software", "offerings", "installed", "items"):
                ids = [e.get("id") if isinstance(e, dict) else e for e in v]
                out.append((path + "." + k, ids))
            walk(v, path + "." + k)
    elif isinstance(x, list):
        for i, v in enumerate(x): walk(v, path + "[%d]" % i)
walk(j, "")
for p, ids in out: print(" ", sys.argv[2], s, p, len(ids), ids[:60])
if not out: print(" ", sys.argv[2], s, json.dumps(j)[:800])
