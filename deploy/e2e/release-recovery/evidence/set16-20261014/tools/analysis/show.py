import json, sys
p = json.load(open(sys.argv[1], encoding='utf-8'))
want = sys.argv[2].split(',')
for ph in p['phases']:
    if ph['phase'] in want:
        print('=====', ph['phase'])
        print(json.dumps(ph, ensure_ascii=False, indent=1)[:int(sys.argv[3]) if len(sys.argv) > 3 else 6000])
