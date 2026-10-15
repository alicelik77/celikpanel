#!/bin/bash
for d in update fresh; do
  A=$(cat /var/tmp/cp-set16/artifacts-$d.path); echo "== $d $A"
  python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]));[print(r,d[r]["version"],d[r]["sequence"],d[r]["commit"],d[r]["sha256"],d[r].get("reused_dist","")) for r in ("baseline","good","defective")]' "$A"
  git -c safe.directory='*' -C "$(dirname $A)/repo" for-each-ref --format='%(refname) %(objectname) %(subject)' refs/upd1
done
ls /var/tmp/cp-pair-accept/dist/; du -sh /var/tmp/cp-set16
