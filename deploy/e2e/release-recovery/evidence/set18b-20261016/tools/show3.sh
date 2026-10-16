#!/bin/bash
source <redacted>
cat $R/native-after-c2.txt; rm -f $R/native-after-c2.txt; echo "$(date -u +%FT%TZ) coordinator native reading after-c2 not taken: the lab had already stopped at 00:17:33Z (file removed)" >> $R/progress.txt
grep '^{"step"' $L/b.out | cut -c1-200; tail -n 3 $L/b.out | cut -c1-400; tail -n 5 $L/b.err | cut -c1-300
d=$(ls -d /var/tmp/cp-release-drill-s18b-a/evidence/*/upd1/*/ | tail -1); echo $d; ls $d $d/steps
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["overall"], [(s["name"],s["verdict"],(s.get("reason") or "")[:120]) for s in d["steps"]])' $d/result.json 2>&1 | head -5
