import json, sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True)
for k, v in db.execute("select key, value from panel_settings where key like '%setup%link%' or key like '%setup%join%' order by key"):
    try:
        d = json.loads(v); shape = {x: ("<set>" if d.get(x) else "") for x in d if any(s in x for s in ("credential", "hash", "ended", "state", "peer_ready", "link_id", "revoked"))}
    except Exception:
        shape = "unparsed"
    print(k, shape)
