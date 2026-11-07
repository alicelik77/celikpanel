import json, sys
t = sys.stdin.read(); i = t.rfind('{"shots"')
if i < 0: print(t[-3000:]); sys.exit()
j = json.loads(t[i:])
print("  shots", j["shots"], "errors", json.dumps(j["errors"])[:600], "native dialogs", json.dumps(j["dialogs"])[:400])
for d in j["deletes"]: print("  UI DELETE", d.get("at"), d.get("apiDelete"), d.get("status"), (d.get("body") or "")[:300])
for p in j["probes"]: print("  probe", p.get("p") or p.get("wt", "")[:80], "ok=%s" % p.get("ok") if p.get("ok") is not None else "", "%ss" % p.get("s") if p.get("s") is not None else "", "reloads", p.get("reloads"), "|", str(p.get("v"))[:1500])
