"""set17b: the driver facts of one lab run (reads only). usage: facts.py LAB"""
import glob, json, sys
run = sorted(glob.glob(f'/var/tmp/cp-release-drill-{sys.argv[1]}/evidence/*/upd1/*/'))[-1]
def step(name):
    hits = sorted(glob.glob(run + f'steps/*-{name}/step.json'))
    return json.load(open(hits[0])) if hits else {}
res = json.load(open(run + 'result.json')) if glob.glob(run + 'result.json') else {}
print('run', run.rstrip('/').split('/')[-1], 'overall', res.get('overall'), 'outcome', (res.get('outcome') or {}).get('classification'))
print('steps', {s['name']: s['verdict'] for s in res.get('steps', [])})
t = step('terminal'); c = t.get('checks') or {}
d = c.get('database') or {}
ledger = d.get('site_file_ledger') or {}
print('final', json.dumps(c.get('final')))
print('builds', {k: (v.get('identity') or '').replace('\n', ' ') for k, v in (c.get('builds') or {}).items()})
print('floor', (c.get('floor') or {}).get('text', '').replace('\n', ' '))
print('database', {k: d.get(k) for k in ('verdict', 'schema_equal', 'tables_compared', 'unexpected', 'volatile_columns_only', 'volatile_excluded', 'differing')})
print('site_file_ledger', {k: ledger.get(k) for k in ('rows_before', 'rows_after', 'rows_added', 'rows_removed', 'changed_columns_by_rowid', 'stable_equal', 'stable_sha256_before', 'stable_sha256_after', 'full_equal', 'full_sha256_before', 'full_sha256_after')})
o = step('owner-start').get('checks') or {}
print('owner_start', json.dumps({k: o.get(k) for k in ('by', 'start_outcome')})[:400], 'status_after', json.dumps((o.get('status_after_the_start') or {}).get('body'))[:300])
a = step('arm').get('checks') or {}
print('arm', json.dumps({k: a.get(k) for k in ('request_id', 'observer_armed')}), 'delay_start', json.dumps({k: (a.get('set17b_restart_delay_start') or {}).get(k) for k in ('started', 'at', 'unit')}))
v = step('verdicts').get('checks') or {}
print('verdicts', step('verdicts').get('verdict'), 'host_panel_windows', json.dumps(v.get('host_panel_windows'))[:500], 'panel', json.dumps(((v.get('workloads') or {}).get('panel') or {}).get('verdict')), 'agreement', json.dumps((v.get('agreement') or {}).get('verdict')))
print('workloads', {k: (w or {}).get('verdict') for k, w in (v.get('workloads') or {}).items()})
tr = step('track'); print('track', tr.get('verdict'), tr.get('started_at'), tr.get('finished_at'), 'samples', (tr.get('checks') or {}).get('samples'))
p = step('post-update-facts'); print('post-update-facts', p.get('verdict'))
r = step('set17-readings-after-the-update').get('checks') or {}
print('readings', json.dumps((r.get('fields') or {}).get('panel_version'))[:400], 'ledger', json.dumps(r.get('ledger'))[:300])
dl = step('set17b-restart-delay'); dc = dl.get('checks') or {}
print('delay', dl.get('verdict'), json.dumps({k: dc.get(k) for k in ('completion_pending_seen', 'frozen', 'thawed', 'held_seconds', 'panel_states_while_held', 'agent_states_while_held', 'panel_active_after_thaw')})[:1500])
print('findings', json.dumps(res.get('findings'))[:600])
