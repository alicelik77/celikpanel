import json, sys
p = json.load(open(sys.argv[1], encoding='utf-8'))
for ph in p['phases']:
    print('==', ph['phase'], ph['started'], ph['seconds'], 'scan_posts', ph['scan_posts'], [(x['url'], x['status'], x['ms']) for x in ph['posts']])
    if ph['phase'] == 'dashboard':
        print(' stale', ph['stale'], 'mount', ph['mount'], 'rescan_link_seen', ph['rescan_link_seen'])
        print(' card', [(c['state'], c['lasted_ms'], (c.get('text') or '')[:70]) for c in ph['card_sequence']])
        print(' mail', [(c['state'], c.get('spinners'), c['lasted_ms'], c.get('old_wording'), (c.get('text') or '')[:120]) for c in ph['mail_sequence']])
        print(' access vfs', [(a['access'] or {}).get('valid_for_seconds') for a in ph['access_reads']])
    if ph['phase'] == 'sidebar':
        for k in ('w1280', 'w390'):
            v = ph[k]; print(' ', k, v.get('id_text'), 'one_line', v.get('id_on_one_line'), 'share', v.get('build_and_id_share_a_line'), 'overflow', v.get('overflows_container'), 'menu', v.get('menu_opened'))
    if ph['phase'] == 'domains':
        print(' first_state_row_shown', ph.get('first_state_row_shown'), 'reads', json.dumps(ph.get('domains_reads'))[:400])
        print(' sidebar', json.dumps(ph.get('sidebar'), ensure_ascii=False)[:300])
    if ph['phase'] == 'components':
        print(' fresh posts', ph['fresh'].get('posts_conditional'), ph['fresh'].get('alerts'))
    if ph['phase'] == 'reads':
        print(' ', json.dumps(ph['reads'], ensure_ascii=False)[:900])
