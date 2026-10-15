#!/bin/bash
lab=/var/tmp/cp-release-drill-$1
d=$(ls -d $lab/evidence/*/upd1/*/ | tail -1); echo "run dir: $d"
ls $d; ls $d/steps | head -40
python3 -I - "$d" <<'PY'
import json, sys, glob, os
d = sys.argv[1]
r = json.load(open(os.path.join(d, 'result.json')))
print('overall', r.get('overall'), 'outcome', r.get('outcome'))
for s in r['steps']: print(' ', s['name'], s['verdict'], (s.get('reason') or '')[:200], s.get('started_at'), s.get('finished_at'))
for f in sorted(glob.glob(os.path.join(d, 'steps', '*readings*', 'step.json'))):
    st = json.load(open(f)); c = st.get('checks') or {}
    print('==', f.split('/steps/')[1]); print(' fields', json.dumps(c.get('fields'))[:900])
    print(' pins', c.get('installed_pins')); print(' ledger', json.dumps(c.get('ledger'))[:500])
    if 'site_file_ledger_digests' in c: print(' sfl', json.dumps(c.get('site_file_ledger_digests'))[:600])
PY
