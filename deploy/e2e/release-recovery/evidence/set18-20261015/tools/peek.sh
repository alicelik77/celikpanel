#!/bin/bash
lab=/var/tmp/cp-release-drill-${1}
d=$(ls -d $lab/evidence/*/upd1/*/ 2>/dev/null | tail -1)
echo "dir=$d"
ls $d/steps 2>/dev/null
f=$(ls -d $d/steps/*${2}*/ 2>/dev/null | head -1)
[ -n "$f" ] && { ls $f; python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps(d, indent=1)[:int(sys.argv[2])])' $f/step.json ${3:-4000}; }
