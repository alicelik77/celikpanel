import json, sys
pre = sys.argv[1]
for line in open('/var/tmp/a114r6/shots/shots.jsonl', encoding='utf-8'):
    try: r = json.loads(line)
    except Exception: continue
    if r.get('prefix') != pre: continue
    if r.get('apiDelete'):
        if 'scan' in r['apiDelete']: continue
        print(' API', r.get('at'), r['apiDelete'], r.get('status'), (r.get('body') or '')[:250].replace('\n', ' '))
    elif r.get('rewritten'): print(' REWRITE', r['rewritten'])
    elif r.get('probe') is not None: print(' ', r.get('name'), r.get('lang'), '|', r.get('probe'), '|', str(r.get('value'))[:700])
    elif r.get('waitJs'): print(' ', r.get('name'), r.get('lang'), '| waitJs ok=%s %ss' % (r.get('ok'), r.get('seconds')))
    elif r.get('error') or r.get('pageerror'): print(' ERR', r.get('name'), r.get('lang'), r.get('error') or r.get('pageerror'))
    elif r.get('dialog'): print(' NATIVE DIALOG', r['dialog'])
