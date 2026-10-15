import glob, json, os, sys
for lab in sys.argv[1:]:
    runs = sorted(glob.glob(f'/var/tmp/cp-release-drill-{lab}/evidence/*/upd1/*/'))
    if not runs: print(lab, 'no run'); continue
    run = runs[-1]
    print('=====', lab, os.path.basename(run.rstrip('/')))
    try:
        r = json.load(open(run + 'result.json'))
    except Exception as e:
        print('no result', e); continue
    print('overall', r.get('overall'), 'outcome', json.dumps(r.get('outcome'))[:400])
    for s in r['steps']:
        print('  ', s['name'], s['verdict'], s.get('started_at'), s.get('finished_at'), (s.get('reason') or '')[:120])
    for d in sorted(glob.glob(run + 'steps/*/step.json')):
        name = d.split('/')[-2]
        st = json.load(open(d))
        c = st.get('checks') or {}
        if name.endswith('owner-start'):
            print('  owner-start', json.dumps({k: c.get(k) for k in ('by', 'start', 'start_outcome', 'status_after_the_start')})[:700])
        if name.endswith('-arm'):
            print('  arm', json.dumps({k: c.get(k) for k in ('request_id', 'observer_armed', 'observer_unit')})[:400], 'page_window' in c)
        if name.endswith('terminal'):
            db = c.get('database') or {}
            led = db.get('site_file_ledger') or {}
            cols = sorted({x for v in (led.get('changed_columns_by_rowid') or {}).values() for x in v})
            print('  terminal db', db.get('verdict'), 'unexpected', db.get('unexpected'), 'volatile_only', db.get('volatile_columns_only'))
            print('  ledger', {k: led.get(k) for k in ('full_equal', 'stable_equal', 'rows_before', 'rows_after', 'rows_added', 'rows_removed')}, 'changed', cols)
            print('  build', json.dumps(c.get('build_identity') or c.get('identity'))[:500])
            print('  floor', json.dumps(c.get('floor'))[:200], 'foundation', json.dumps(c.get('foundation'))[:300])
            print('  tables', db.get('tables_compared'), 'excluded', list((db.get('volatile_reasons') or {}).keys()) if isinstance(db.get('volatile_reasons'), dict) else db.get('volatile_reasons'))
