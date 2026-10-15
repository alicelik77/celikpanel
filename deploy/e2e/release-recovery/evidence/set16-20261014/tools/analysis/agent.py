import json, sys
p = json.load(open(sys.argv[1], encoding='utf-8'))
ph = [x for x in p['phases'] if x['phase'] == 'agent'][0]
def surf(name, s):
    print('---', name, 'found', s.get('found'), 'danger', s.get('danger_rgb'), s.get('error'))
    for a in s.get('alerts') or []:
        print('  alert fw=%s border=%s bg=%s color=%s failure_on_alert=%s in_failure=%s width=%s tokens=%s unbroken=%s runs=%s' % (a['data_firewall_unread'], a['border_color'], a['background_color'], a['color'], a['failure_colour_on_alert'], a['elements_in_failure_colour'], a['width'], a['celikpanel_agent_tokens'], a['celikpanel_agent_all_unbroken'], a['token_runs']))
        print('   text:', a['text'][:300])
print('stop', json.dumps(ph['stop'], ensure_ascii=False)[:400])
surf('components_tr (desktop)', ph['components_tr_surface'])
c = ph['components_tr']
print('components_tr', {k: c.get(k) for k in ('load_at', 'agent_sentence_tr', 'agent_sentence_en', 'html_lang')})
print(' firewall_lines', c.get('firewall_lines'))
print(' alerts', [x[:200] for x in c.get('alerts') or []])
print(' firewall_reads', c.get('firewall_reads'))
print(' scan_posts', c.get('scan_posts'))
surf('components_tr phone', ph['components_tr_phone_surface'])
for tag in ('tr', 'en'):
    d = ph['dashboard_' + tag]
    print('=== dashboard', tag, d['load_at'], 'names_agent', d['attention_names_agent'], 'other_lang_sentence', d['agent_sentence_other_language'], 'states', d['states'], 'mail', d['mail_states'])
    print(' attention', json.dumps(d['attention'], ensure_ascii=False)[:700])
    surf('attention surface ' + tag, d['attention_surface'])
    print(' scan_posts', d['scan_posts'])
    print(' agent_reads', d['agent_reads'][:8])
print('start', json.dumps(ph['start'], ensure_ascii=False)[:300])
a = ph['components_tr_after_rescan']
print('after rescan', {k: a.get(k) for k in ('pressed', 'press_at', 'agent_sentence_tr')}, a.get('firewall_lines'), [x[:150] for x in a.get('alerts') or []])
print(' fw reads after press', a.get('firewall_reads_after_press'))
print(' scan posts after press', a.get('scan_posts_after_press'))
