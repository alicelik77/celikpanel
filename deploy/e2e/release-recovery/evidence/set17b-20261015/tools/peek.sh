#!/bin/bash
source <redacted>
python3 -I -c '
import json,sys
for n in sys.argv[1:]:
    d=json.load(open(n)); u=d.get("update") or {}
    print(n.split("/")[-1], d.get("schema"), d.get("cell"), d.get("base_cell"), d.get("baseline_ref"), d.get("update_page_window"), d.get("source_commit"), d.get("lab_preparation"), d.get("not_run"))
    print("  ", json.dumps({k:u.get(k) for k in ("baseline","candidate") if k in u})[:500])
' $R/build/dry-*.json
