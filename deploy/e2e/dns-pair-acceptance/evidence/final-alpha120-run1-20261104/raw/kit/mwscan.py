import json, sys
d = json.load(sys.stdin)
try:
    b = json.loads(d["body"])
except Exception:
    print("http", d.get("status"), (d.get("body") or "")[:200]); sys.exit()
if len(sys.argv) > 1:
    for f in b.get("findings") or []:
        print("  finding", f.get("id"), f.get("path"), f.get("signature"), f.get("state"))
    sys.exit()
sc = (b.get("scans") or [{}])[0]
print(sc.get("status"), "files", sc.get("files_scanned"), "infected", sc.get("infected"), sc.get("code"), (sc.get("detail") or "")[:200])
