#!/bin/bash
source <redacted>
for d in update second; do
  ART=$(cat $R/artifacts-$d.path)
  python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]));[print(sys.argv[2],r,d[r]["version"],d[r]["sequence"],d[r]["commit"],d[r]["sha256"],d[r]["dist_json"],d[r].get("reused_dist","")) for r in ("baseline","good","defective")]' "$ART" $d
done
ls -la /var/tmp/cp-pair-accept/dist/
du -sh /var/tmp/cp-pair-accept/dist/* 2>/dev/null
