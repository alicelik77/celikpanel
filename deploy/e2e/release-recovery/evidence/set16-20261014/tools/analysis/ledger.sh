#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1); echo "run dir $d"
ls $d/steps
find $d -name '*managed-site-files*' | sed "s#$d##"
python3 -I - "$d" <<'PY'
import json, sys, os, glob
d = sys.argv[1]
sys.path.insert(0, '/var/tmp/cp-set16/harness-c/deploy/e2e/release-recovery')
f = glob.glob(os.path.join(d, 'steps', '*-terminal', 'step.json'))[0]
st = json.load(open(f)); c = st.get('checks') or {}
def find(o, key, path=''):
    if isinstance(o, dict):
        for k, v in o.items():
            if k == key: yield path + '/' + k, v
            yield from find(v, key, path + '/' + k)
    elif isinstance(o, list):
        for i, v in enumerate(o): yield from find(v, key, f'{path}[{i}]')
print('terminal verdict', st.get('verdict'), 'keys', list(c.keys())[:30])
for p, v in find(c, 'site_file_ledger'):
    print('SFL at', p); print(json.dumps(v, indent=1)[:2500])
for p, v in find(c, 'database'):
    if isinstance(v, dict) and 'verdict' in v:
        print('DB at', p, {k: v.get(k) for k in ('verdict', 'unexpected', 'volatile_columns_only', 'schema_equal')})
        import set16_trial
        print('ledger_summary', json.dumps(set16_trial.ledger_summary(v)))
PY
