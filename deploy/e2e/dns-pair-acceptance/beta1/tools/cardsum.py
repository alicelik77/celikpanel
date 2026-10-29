import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"])
print(d["status"], b.get("checked_at"), "parent_ns=", b.get("parent_ns"), [(i["id"], i["state"]) for i in b.get("items",[])], "servers=", [(s["role"], s["state"]) for s in b.get("servers",[])])
