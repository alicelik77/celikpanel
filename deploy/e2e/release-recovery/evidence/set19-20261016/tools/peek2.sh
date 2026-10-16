#!/bin/bash
# set19: read-only: the preparation text after the change, and the terminal step's findings. usage: peek2.sh LABNAME
E=$(find /var/tmp/cp-release-drill-$1/evidence -maxdepth 4 -type d -name 'upd1-debian13-good-*' | head -1)
ls $E/steps/11-set19-certbot-lab-preparation $E/steps/16-terminal
awk '/== after/{f=1} f' $E/steps/11-set19-certbot-lab-preparation/set19-certbot-preparation.txt | head -34
python3 - $E/steps/16-terminal/step.json <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
print(d.get('verdict'), d.get('reason'))
c=d.get('checks') or {}
print(sorted(c.keys())[:60])
for k in ('firewall','firewall_ruleset','renewal','timers','scheduler','status'):
    if k in c: print(k, json.dumps(c[k])[:800])
PY
