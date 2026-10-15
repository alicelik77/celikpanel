#!/bin/bash
source <redacted>
python3 -I -c '
import json,sys
for n in sys.argv[1:]:
    d=json.load(open(n)); u=d.get("update") or {}
    print(n.split("/")[-1], d.get("cell_kind"), d.get("baseline_ref"), d.get("update_page_window"), json.dumps({k:u.get(k) for k in ("baseline","candidate","labels","provenance") if k in u})[:600])
' $R/build/dry-*.json
cat $R/build/update-prove.json | head -c 600; echo
cat $R/build/second-prove.stderr.txt | tail -3
