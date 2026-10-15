import json, sys
p = json.load(open(sys.argv[1], encoding='utf-8'))
ph = [x for x in p['phases'] if x['phase'] == 'clock'][0]
for s in ph['steps']:
    print('STEP', s['what'], s['at'], json.dumps(s['value'], ensure_ascii=False)[:600])
for k in ('ahead', 'behind'):
    v = ph[k]
    print('=====', k)
    for key in ('clock_before', 'clock_after'):
        print(key, json.dumps(v.get(key), ensure_ascii=False)[:500])
    print('access_offsets_ms', v.get('access_offsets_ms'))
    print('mounted', v.get('mounted'), 'other_api_reads', v.get('other_api_reads'), 'access_reads_whole_page', v.get('access_reads_whole_page'), 'old_rule_would_refuse', v.get('old_rule_would_refuse'))
    print('states', v.get('states'), 'mount', v.get('mount'), 'posts_conditional', v.get('posts_conditional'))
    for a in v.get('access_reads') or []:
        ac = a.get('access') or {}
        print('  access', a.get('start'), a.get('status'), 'can_use', ac.get('can_use_panel'), 'vfs', ac.get('valid_for_seconds'), 'old_rule_ok', ac.get('old_rule_accepts_at_receipt'), 'g-b', ac.get('guest_minus_browser_ms'), 'deadline', ac.get('browser_deadline'))
    print('card', [(c.get('state'), c.get('lasted_ms'), (c.get('text') or '')[:60]) for c in v.get('card_sequence') or []])
print('restored', json.dumps(ph.get('restored'), ensure_ascii=False)[:600])
