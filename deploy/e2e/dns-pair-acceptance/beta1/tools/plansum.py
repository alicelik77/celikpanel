import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else d["body"]
if isinstance(b,str): print(d["status"], b[:500]); sys.exit()
print(d["status"], "can_start", b.get("can_start"), "blockers", b.get("blockers"))
print("steps", [s["id"]+":"+s.get("target","") for s in b.get("steps",[])])
print("package_updates_state", b.get("package_updates_state"), [ (u.get("package"),u.get("from"),u.get("to"),u.get("reason")) for u in (b.get("package_updates") or [])])
