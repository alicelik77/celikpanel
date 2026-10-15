import json, sys
s = json.load(open(sys.argv[1], encoding='utf-8'))
t = s['timeline']
print({k: s.get(k) for k in ('start_pressed_at', 'bounded_end')})
print('release', s.get('release'))
a = s.get('start_answer') or {}
print('answer', {k: a.get(k) for k in ('status', 'answer_seen_by_the_browser', 'page_dialog_line_after_release')})
for k in ('samples', 'sample_errors', 'availability_outages', 'availability_longest_outage_s', 'page_status_reads', 'page_status_reads_lost',
          'page_first_lost_read', 'page_first_answer_after_loss', 'page_outage_s', 'version_changed', 'not_known_wording', 'restarting_wording',
          'corner_notice', 'warning_surface', 'alerts_seen'):
    print(k, json.dumps(t.get(k), ensure_ascii=True)[:420])
print('windows', json.dumps(t['availability_windows'])[:900])
for st in t['states']:
    d = st['dialog'] or {}
    print(st['from_s'], st['to_s'], st['samples'], st['dialog_kind'], '|', d.get('title'), '|', (d.get('status') or '')[:150], '| lock', bool(d.get('lock_line')),
          '| corner', st['corner_present'], st['corner_kind'], '| card', st['card_phase'], '| warn', st['warning_surface'],
          json.dumps(st['surface']['dialog_icon']), 'cardicon', (st['surface']['card'] or {}).get('icon_color'), (st['surface']['card'] or {}).get('svg'),
          'T+', d.get('t_plus_first'), st.get('t_plus_last'), st['sidebar_version'], 'aria-hidden', st['app_aria_hidden'])
