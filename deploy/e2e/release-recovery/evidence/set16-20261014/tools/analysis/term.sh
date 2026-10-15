#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1)
for f in $d/steps/10-pre-state/*managed-site-files.jsonl $d/steps/15-terminal/*managed-site-files.jsonl; do echo "== $(basename $f)"; cut -c1-600 $f; done
python3 -I - "$d" <<'PY'
import json, sys, os, glob
d = sys.argv[1]
c = json.load(open(glob.glob(os.path.join(d, 'steps', '*-terminal', 'step.json'))[0]))['checks']
for k in ('outcome', 'final', 'floor', 'builds', 'build_identity_ok', 'running_matches_installed', 'transaction', 'update_card'):
    print(k, json.dumps(c.get(k))[:700])
r = json.load(open(os.path.join(d, 'result.json')))
print('RESULT overall', r.get('overall'), 'outcome', json.dumps(r.get('outcome'))[:600])
print('fault', json.dumps(r.get('fault') or r.get('injection') or (r.get('cell') or {}))[:500])
for s in r['steps']: print(' ', s['name'], s['verdict'], (s.get('reason') or '')[:160], s.get('started_at'), s.get('finished_at'))
PY
