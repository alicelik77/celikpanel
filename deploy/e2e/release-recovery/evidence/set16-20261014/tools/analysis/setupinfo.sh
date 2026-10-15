#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1)
python3 -I - "$d" <<'PY'
import json, sys, os
d = sys.argv[1]
st = json.load(open(os.path.join(d, 'steps', '07-setup', 'step.json')))
c = st.get('checks') or {}
print(list(c.keys()))
for k in ('waiting', 'phase', 'final', 'setup_waiting', 'summary'):
    if k in c: print(k, json.dumps(c[k])[:800])
w = json.load(open(os.path.join(d, 'steps', '09-set16-browser-window', 'step.json'))).get('checks') or {}
print('leave_setup', json.dumps(w.get('leave_setup'))[:800])
print('clock_at_the_end', json.dumps(w.get('clock_at_the_end'))[:500])
print('prep_count', w.get('prep_count'))
PY
