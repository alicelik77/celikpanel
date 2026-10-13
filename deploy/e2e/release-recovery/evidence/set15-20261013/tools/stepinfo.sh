#!/bin/bash
# set15: print one step's step.json (bounded) of a lab. usage: stepinfo.sh LAB NODE STEP-SUBSTRING [CHARS]
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/$2/upd1/*/ | tail -1)
f=$(ls -d $d/steps/*$3* | head -1)
echo "## $f"; ls $f
python3 -I -c "import json,sys;d=json.load(open(sys.argv[1]));print(json.dumps(d,indent=1)[:int(sys.argv[2])])" $f/step.json ${4:-3000}
