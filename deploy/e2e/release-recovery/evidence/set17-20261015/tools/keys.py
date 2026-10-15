import glob, json, sys
run = sorted(glob.glob(f'/var/tmp/cp-release-drill-{sys.argv[1]}/evidence/*/upd1/*/'))[-1]
for d in sorted(glob.glob(run + 'steps/*/step.json')):
    st = json.load(open(d)); c = st.get('checks') or {}
    print(d.split('/')[-2], sorted(c.keys())[:40])
