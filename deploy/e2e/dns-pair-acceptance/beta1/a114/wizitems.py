# usage: b1 api CELL NODE GET /api/v1/setup | python3 wizitems.py OUT.json STEP...
# -> screenshot items that reopen the wizard at each STEP with the saved draft (the editor checkpoint).
import json, sys
d = json.load(sys.stdin); snap = json.loads(d["body"])
items = [{"name": f"wizard-{s}", "path": "/setup", "checkpoint": {"revision": snap["revision"], "step": s, "draft": snap["draft"]}, "wait": 1500} for s in sys.argv[2:]]
json.dump(items, open(sys.argv[1], "w"))
print("items", [i["name"] for i in items], "revision", snap["revision"], "status", snap.get("status"))
