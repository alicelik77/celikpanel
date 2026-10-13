#!/bin/bash
D=$(ls -d /var/tmp/cp-release-drill-s14b-d13-a/evidence/debian13/upd1/*/ | tail -1)
echo "$D"; ls $D; ls $D/steps/08-set14-browser-window/ | head; ls $D/steps/08-set14-browser-window/native | tail -12
cat $D/steps/08-set14-browser-window/clock-at-the-end.json | head -c 1500; echo
J=$(ls $D/steps/08-set14-browser-window/native/*journal* 2>/dev/null | head -1); echo "journal file: $J"
python3 -I -c '
import json,sys
v=json.load(open(sys.argv[1]))
print(list(v.keys())[:20])
' "$J" 2>&1 | head -5
