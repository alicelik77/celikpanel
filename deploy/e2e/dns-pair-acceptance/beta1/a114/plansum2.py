# usage: plansum2.py PLANFILE LABEL
import json, sys
d = json.load(open(sys.argv[1]))
b = json.loads(d["body"]) if d["body"].startswith("{") else {}
print(sys.argv[2], d["status"], "can_start", b.get("can_start"), "blockers", json.dumps(b.get("blockers"))[:700],
      [s["id"] + ":" + str(s.get("target", "")) for s in b.get("steps", [])], "" if b else d["body"][:800])
