import json, sys
s = json.load(open(sys.argv[1], encoding='utf-8'))
for n, p in s['phases'].items():
    if 'error' in p: print(n, 'ERROR', p['error'][:600]); continue
    if n == 'sidebar': print(n, {k: {x: p[k].get(x) for x in ('id_text', 'id_on_one_line', 'stamp_line', 'overflows_container', 'menu_opened')} for k in ('w1280', 'w390')})
    if n == 'components':
        for l in ('en', 'tr'): q = p[l]; print(n, l, {k: q[k] for k in ('html_lang', 'rows', 'catalogue_ids', 'rows_in_catalogue', 'names_match', 'descriptions_match', 'catalogue_ids_not_on_page', 'rows_not_in_catalogue')}, 'differ', q['names_differ'][:3], q['descriptions_differ'][:3])
    if n == 'dashboard':
        for l in ('en', 'tr'): q = p[l]; print(n, l, q['html_lang'], q['section_found'], json.dumps(q['items'], ensure_ascii=True), 'RAW', q['items_with_a_raw_id'], q['items_with_an_underscore_word'], q['audit_actions_read'][:12])
    if n == 'store':
        for l in ('en', 'tr'): q = p[l]; print(n, l, q.get('path'), q.get('html_lang'), json.dumps(q.get('badge_counts'), ensure_ascii=True))
    if n == 'component-unverified':
        for l in ('en', 'tr'):
            q = p[l]; print(n, l, q['id'], q['html_lang'], 'prep', json.dumps(q['preparation'])[:300])
            for x in q['sequence']: print('   ', x['after_ms'], x['kinds'], x['alerts'], x['text_head'][:220].encode('ascii', 'replace').decode())
            for r in q['requests']: print('    req', r['method'], r['url'][:60], r['status'], r['start'][11:23], r['ms'], (r.get('body') or '')[:200])
    if n == 'reads': print(n, json.dumps({k: v.get('status') for k, v in p['reads'].items()}), json.dumps(p['reads'].get('/api/v1/panel/version', {}).get('body'))[:300])
