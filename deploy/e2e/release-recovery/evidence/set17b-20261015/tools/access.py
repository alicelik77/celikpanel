"""set17b: the page's own reads by path around the outage (reads only). usage: access.py page-dir"""
import json, sys
from collections import Counter
d = sys.argv[1]
s = json.load(open(d + '/page-summary.json', encoding='utf-8'))
pressed = s['timeline']['pressed_at']
import datetime
p = datetime.datetime.fromisoformat(pressed.replace('Z', '+00:00')).timestamp() * 1000
reqs = [json.loads(l) for l in open(d + '/requests.jsonl', encoding='utf-8') if l.strip()]
reqs = [r for r in reqs if r.get('page') == 'update-page']
rel = lambda ms: round((ms - p) / 1000, 1)
paths = Counter(r['url'].split('?')[0] for r in reqs if r['start_ms'] >= p)
print('paths after press', dict(paths))
for path in ('/api/v1/license/access', '/api/v1/auth/me', '/api/v1/panel/update/status', '/api/v1/panel/availability', '/api/v1/recovery/status'):
    rows = [r for r in reqs if r['url'].split('?')[0] == path]
    before = [r for r in rows if r['start_ms'] < p + 25000 and r['status'] and r['status'] < 500]
    lost = [r for r in rows if r['start_ms'] >= p and (r['status'] is None or r['status'] >= 500)]
    good_after = [r for r in rows if lost and r['start_ms'] > lost[0]['start_ms'] and r['status'] and r['status'] < 500]
    print(path, 'reads', len(rows), '| last good before T+25', (rel(before[-1]['start_ms']), before[-1]['status']) if before else None,
          '| lost', len(lost), (rel(lost[0]['start_ms']), rel(lost[-1]['start_ms'])) if lost else None,
          '| first good after loss', (rel(good_after[0]['start_ms']), good_after[0]['status']) if good_after else None,
          '| codes', dict(Counter(r.get('code') for r in rows if r.get('code'))), '| errors', dict(Counter(r.get('error') for r in lost)))
gaps = []
sr = sorted([r for r in reqs if r['url'].startswith('/api/v1/panel/update/status') and r['start_ms'] >= p], key=lambda r: r['start_ms'])
for a, b in zip(sr, sr[1:]):
    gaps.append(round((b['start_ms'] - a['start_ms']) / 1000, 1))
print('status read intervals during the run: min', min(gaps) if gaps else None, 'max', max(gaps) if gaps else None, 'median', sorted(gaps)[len(gaps)//2] if gaps else None)
