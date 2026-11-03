import json, sys
d = json.load(sys.stdin)
try:
    b = json.loads(d["body"])
except Exception:
    print("http", d.get("status"), (d.get("body") or "")[:200]); sys.exit()
i = b.get("install") or {}
o = b.get("operation") or {}
print("install=%s active=%s serving=%s unit=%s url=%s | op=%s step=%s class=%s reason=%s msg=%s | suspended=%s cert=%s" % (
    i.get("state"), i.get("active"), i.get("serving"), i.get("unit"), i.get("url"), o.get("state"), o.get("step"),
    o.get("class"), o.get("reason"), (o.get("message") or "")[:200], b.get("suspended"), json.dumps(b.get("certificate"))[:120]))
