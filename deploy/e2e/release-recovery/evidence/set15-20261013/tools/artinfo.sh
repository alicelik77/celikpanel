#!/bin/bash
for d in update fresh; do ART=$(cat /var/tmp/cp-set15-run/artifacts-$d.path); echo "## $d $ART"
python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]))
for r in ("baseline","good","defective"): b=d[r]; print(r,b["version"],b["sequence"],b["commit"],b["tree"],b["sha256"],b.get("reused_dist",""))' "$ART"; done
