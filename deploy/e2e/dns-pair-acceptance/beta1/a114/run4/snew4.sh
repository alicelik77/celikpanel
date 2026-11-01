python3 - <<'PY'
import json
rows=[json.loads(l) for l in open("/var/tmp/a114r4/shots/shots.jsonl") if l.strip()]
for r in rows:
    if str(r.get("prefix","")).startswith("snew4") or "snew4" in str(r.get("file","")) or r.get("probe"):
        print(json.dumps({k:r.get(k) for k in ("prefix","name","lang","width","file","sw","iw","probe","label","value")}, ensure_ascii=False)[:500])
PY
ls /var/tmp/a114r4/shots | head -30
