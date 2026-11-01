# usage: b1 api CELL backup GET /api/v1/setup/operation | python3 e1watch.py  -> one line about the publisher step + flags
import json, re, sys
d = json.load(sys.stdin)
raw = d.get("body") or ""
try:
    b = json.loads(raw)
except Exception:
    print("http", d.get("status"), raw[:200]); sys.exit()
op = b.get("operation", b) if isinstance(b, dict) else {}
steps = op.get("steps", []) or []
pub = next((s for s in steps if s.get("id") == "dns_publisher" or s.get("kind") == "dns_publisher"), None)
flags = []
if "_refused" in raw: flags.append("RAW_HAS__refused")
if re.search(r"new (join )?code|create a new", raw, re.I): flags.append("RAW_SAYS_NEW_CODE")
cur = next((s.get("id") for s in steps if s.get("status") not in ("succeeded", "pending", "completed", "skipped")), None)
keep = {k: pub.get(k) for k in pub if k not in ("id",)} if pub else None
print(op.get("status"), "cur=" + str(cur), "| publisher:", json.dumps(keep)[:900] if keep else None, "| error:", json.dumps(op.get("error"))[:400], "|", " ".join(flags))
