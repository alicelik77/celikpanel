"""set14c: print the card sequence and the scan POSTs of a browser output directory (ui/)."""
import json, sys
from pathlib import Path
d = Path(sys.argv[1]); only = sys.argv[2] if len(sys.argv) > 2 else None
last = {}
for l in open(d / "cardlog.jsonl", encoding="utf-8"):
    r = json.loads(l)
    if only and r.get("phase") != only: continue
    if r.get("ev"): print(f"{r['at'][11:23]} {r['page']:<14} {r['phase']:<11} [{r['ev']} {r.get('vis')}]"); continue
    print(f"{r['at'][11:23]} {r['page']:<14} {r['phase']:<11} {r.get('vis','')[:3]} {r.get('state'):<10} {(r.get('text') or '')[:110]}")
print("--- scan POSTs")
for l in open(d / "requests.jsonl", encoding="utf-8"):
    r = json.loads(l)
    if r["method"] == "POST" and "managed-services/scan" in r["url"] and (not only or r["phase"] == only):
        print(r["start"][11:23], r["end"][11:23], r["page"], r["phase"], r["url"], r["status"], r["ms"], json.dumps(r.get("answer")))
