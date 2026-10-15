import json, sys, glob, os
E = sys.argv[1]
for g in sys.argv[2:]:
    f = glob.glob(os.path.join(E, g, 'driver', 'steps', '*-terminal', 'step.json'))[0]
    c = json.load(open(f, encoding='utf-8'))['checks']
    db = c.get('database') or {}
    sfl = db.get('site_file_ledger') or {}
    cols = sorted({x for v in (sfl.get('changed_columns_by_rowid') or {}).values() for x in v})
    print(g, 'verdict', db.get('verdict'), 'unexpected', db.get('unexpected'), 'volatile_only', db.get('volatile_columns_only'),
          'full_equal', sfl.get('full_equal'), 'stable_equal', sfl.get('stable_equal'), 'changed', cols,
          'rows', sfl.get('rows_before'), sfl.get('rows_after'), 'stable', (sfl.get('stable_sha256_before') or '')[:16], (sfl.get('stable_sha256_after') or '')[:16],
          'full', (sfl.get('full_sha256_before') or '')[:16], (sfl.get('full_sha256_after') or '')[:16])
    print('   other db keys', [k for k in db.keys()][:20])
    o = json.load(open(os.path.join(E, g, 'driver', 'result.json'), encoding='utf-8'))['outcome']
    print('   outcome keys', list(o.keys()))
    for k in ('fault', 'injected_fault', 'reboot', 'owner_reboot', 'final_status', 'floor'):
        if k in o: print('  ', k, json.dumps(o[k])[:400])
    print('   floor', json.dumps(c.get('floor'))[:200]); print('   builds', json.dumps({k: v.get('identity') for k, v in (c.get('builds') or {}).items()}))
