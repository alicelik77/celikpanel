#!/bin/bash
# set17b: print a lab's restart-delay records (reads only). usage: delayrec.sh LAB
run=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1)
ls $run/steps/ | tr '\n' ' '; echo
d=$(ls -d $run/steps/*set17b-restart-delay* 2>/dev/null | head -1)
echo "== $d"; ls $d
python3 -I -c 'import json,sys;s=json.load(open(sys.argv[1]));print(s.get("verdict"),s.get("reason"));c=s.get("checks") or {};print(json.dumps({k:c.get(k) for k in ("kinds","checkpoint_seen","frozen","thawed","held_seconds","released")},indent=1)[:3000])' $d/step.json
cat $d/units-at-collection.txt 2>/dev/null | head -40
cut -c1-400 $d/set17b-restart-delay-events.jsonl 2>/dev/null
a=$(ls -d $run/steps/*-arm 2>/dev/null | head -1)
python3 -I -c 'import json,sys;s=json.load(open(sys.argv[1]));c=s.get("checks") or {};print(json.dumps(c.get("set17b_restart_delay_start"))[:1500])' $a/step.json
