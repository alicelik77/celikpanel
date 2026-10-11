import json, sys, glob, os
root = sys.argv[1]; only = sys.argv[2:] 
for f in sorted(glob.glob(root + "/steps/*/section.json")):
    d = json.load(open(f))
    for name, c in (d.get("cells") or {}).items():
        if only and name not in only: continue
        print(f"== {os.path.basename(os.path.dirname(f))} {name}: {c['verdict']}")
        for e in c["expectations"]:
            if e["held"] is not True or only:
                print("  [%s] %s" % (e["held"], e["expectation"][:150]))
                print("     ", json.dumps(e["measured"], default=str)[:1500])
    if d.get("error"): print("ERROR", os.path.basename(os.path.dirname(f)), d["error"][:800])
