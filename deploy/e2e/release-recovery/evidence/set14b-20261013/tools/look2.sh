#!/bin/bash
D=$(ls -d /var/tmp/cp-release-drill-s14b-d13-a/evidence/debian13/upd1/*/ | tail -1)
J=$D/steps/08-set14-browser-window/native/028-ui-journal-27-journal.json
python3 -I - "$J" <<'PY'
import json,sys,re
v=json.load(open(sys.argv[1]))
for unit,rows in (v.get("by_unit") or {}).items():
    rows = rows if isinstance(rows,list) else rows.get("lines", [])
    print("==",unit,len(rows))
    for r in rows:
        t = r if isinstance(r,str) else json.dumps(r)[:400]
        if re.search(r"licen|503|500|agent|clock|time|verif|socket", t, re.I): print("  ", t[:330])
PY
