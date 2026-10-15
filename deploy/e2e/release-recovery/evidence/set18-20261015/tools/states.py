import json,sys
for l in open(sys.argv[1], encoding="utf-8"):
    d=json.loads(l)
    if d.get("new_state") or d.get("marker"):
        import datetime
        t=datetime.datetime.utcfromtimestamp(d["t"]/1000).strftime("%H:%M:%S") if d.get("t") else "?"
        print(t, d.get("progress_title"), "|", d.get("guidance_title"), "|", (d.get("operation") or {}).get("phase"), (d.get("operation") or {}).get("status"), "| fin:", d.get("finishing"), "| rec:", (d.get("reconnecting") or "")[:60], "| steps:", "".join(s["state"][0] for s in d.get("steps",[])), "|", d.get("h1"), "| checks:", [(c["id"],c["state"]) for c in d.get("checks",[])])
