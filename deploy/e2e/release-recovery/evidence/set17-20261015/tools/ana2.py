import glob, json, sys
def step(run, suffix):
    for d in sorted(glob.glob(run + 'steps/*/step.json')):
        if d.split('/')[-2].endswith(suffix): return json.load(open(d))
    return {}
for lab in sys.argv[1:]:
    run = sorted(glob.glob(f'/var/tmp/cp-release-drill-{lab}/evidence/*/upd1/*/'))[-1]
    print('=====', lab, run.split('/')[-2])
    r = json.load(open(run + 'result.json'))
    print('overall', r['overall'], 'classification', (r.get('outcome') or {}).get('classification'), 'final', json.dumps((r.get('outcome') or {}).get('final_status'))[:300])
    tr = step(run, 'track').get('checks') or {}
    f = tr.get('final') or {}
    print('track final', {k: f.get(k) for k in ('phase', 'reason', 'previous_failure', 'terminal_proof', 'status', 'operation', 'finished_at', 'updated_at')}, 'samples', tr.get('samples'), 'agreement', json.dumps(tr.get('agreement'))[:200])
    print('reboot', json.dumps(tr.get('reboot'))[:500])
    arm = step(run, '-arm'); st = step(run, 'owner-start')
    print('arm', arm.get('started_at'), arm.get('finished_at'), 'start', st.get('started_at'), json.dumps((st.get('checks') or {}).get('start'))[:200])
    te = (step(run, 'terminal').get('checks') or {})
    print('terminal builds', json.dumps(te.get('builds'))[:600])
    print('terminal outcome', json.dumps(te.get('outcome'))[:300], 'login', te.get('login_ok'), 'marker', json.dumps(te.get('site_marker'))[:120], 'firewall', te.get('firewall_equal'), 'running_matches_installed', te.get('running_matches_installed'))
    db = te.get('database') or {}; led = db.get('site_file_ledger') or {}
    print('db', db.get('verdict'), db.get('unexpected'), db.get('volatile_columns_only'), {k: led.get(k) for k in ('full_equal', 'stable_equal', 'rows_before', 'rows_after', 'rows_added', 'rows_removed', 'changed_columns_by_rowid')})
    print('db keys', sorted(db.keys())[:30])
    print('floor', (te.get('floor') or {}).get('text'), 'foundation', ((te.get('foundation') or {}).get('text') or '')[:260].replace('\n', ' | '))
    v = step(run, 'verdicts').get('checks') or {}
    print('host_panel_windows', json.dumps(v.get('host_panel_windows'))[:600])
    print('workloads panel', json.dumps((v.get('workloads') or {}).get('panel'))[:600])
    print('attempts', json.dumps(v.get('attempts'))[:500])
    for jl in sorted(glob.glob(run + 'steps/*/*managed-site-files.jsonl')):
        first = open(jl).readline().strip()
        rows = open(jl).read().splitlines()[1:]
        print('ledger file', jl.split('steps/')[1], first[:300], '| row observed_at', [json.loads(x).get('observed_at') if x.startswith('{') else x[:80] for x in rows][:3])
print('##### ledger digests')
for lab in sys.argv[1:]:
    run = sorted(glob.glob(f'/var/tmp/cp-release-drill-{lab}/evidence/*/upd1/*/'))[-1]
    for jl in sorted(glob.glob(run + 'steps/*/*managed-site-files.jsonl')):
        led = json.loads(open(jl).readline()).get('ledger', {})
        print(lab, jl.split('steps/')[1].split('/')[0], 'stable', led.get('stable_sha256'), 'full', led.get('full_sha256'), 'rows', led.get('rows'))
