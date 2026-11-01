#!/bin/bash
# run 3 screens on topology A: S-new-1 (joined ready page + dashboard), S-new-2 (Add domain), S-new-3 (Audit log),
# the forgot-password command (S23 source, then run in a root terminal). EN/TR, desktop and 390 px. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
CELL=a5-a
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/shotsA.log) 2>&1
I=/var/tmp/a114r3/items; mkdir -p $I
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); [json.dump(v, open(sys.argv[2]+"/"+k+".json","w")) for k,v in d.items()]' $H/shotsA-items.json $I
echo "== S-new-1: joined server ready page and dashboard $(ts)"
$SH $CELL backup snew1 $I/snew1.json
echo "== S-new-2: Add domain names the DNS software (first and joined server) $(ts)"
for n in main backup; do $SH $CELL $n snew2-$n $I/snew2.json; done
echo "== S-new-3: Audit log in words $(ts)"
$SH $CELL main snew3 $I/snew3.json
echo "== forgot-password page: the command shown (S23 source) $(ts)"
$SH $CELL main s23 $I/s23.json --no-login
python3 - <<'PY'
import json
rows=[json.loads(l) for l in open("/var/tmp/a114r3/shots/shots.jsonl") if l.strip()]
v=[r["value"] for r in rows if r.get("prefix")=="s23" and r.get("probe")=="owner command" and r.get("lang")=="en" and r.get("width")=="desktop"]
open("/var/tmp/a114r3/s23-screen-command.txt","w").write((v[-1] if v else "").strip()+"\n")
print("screen command:", v[-1] if v else None)
PY
echo "== S23: the screen's command in a root terminal $(ts)"
python3 -I $H/owner.py $CELL main forgotcmd /var/tmp/a114r3/s23-screen-command.txt 2>&1 | grep -E '^PASS|^FAIL' | cut -c1-1500
echo "== screens done $(ts)"
