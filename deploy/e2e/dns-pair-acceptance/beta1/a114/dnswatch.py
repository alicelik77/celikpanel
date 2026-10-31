# usage: b1 api ... GET /api/v1/dns/overview | python3 dnswatch.py   -> one line: sentence, servers, not-ok names
import json, sys
d = json.load(sys.stdin)
b = json.loads(d["body"]) if (d.get("body") or "").startswith("{") else {}
s = b.get("sentence") or {}
servers = [(x.get("who"), x.get("role"), x.get("order"), x.get("short_name")) for x in b.get("servers", [])]
names = [(n.get("name"), n.get("kind"), n.get("type"), n.get("state"), n.get("optional")) for n in b.get("names", []) if n.get("state") not in ("ok", "served", None)]
print(s.get("code"), s.get("state"), "who=" + str(s.get("who")), "|", servers, "| not-ok:", names[:6])
