"""set18b: print selected events (by ev name or view label) of events.jsonl, compactly. usage: ev.py FILE [EV_OR_LABEL ...] [--chars N]"""
import json, sys

args = sys.argv[2:]
n = 1500
if "--chars" in args:
    i = args.index("--chars"); n = int(args[i + 1]); del args[i:i + 2]
for line in open(sys.argv[1], encoding="utf-8"):
    d = json.loads(line)
    if args and d.get("ev") not in args and d.get("label") not in args and d.get("name") not in args:
        continue
    print(json.dumps(d, ensure_ascii=False)[:n])
