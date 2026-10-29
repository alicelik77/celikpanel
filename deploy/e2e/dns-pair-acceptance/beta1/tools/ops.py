import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True)
cols = [r[1] for r in db.execute("pragma table_info(service_operations)")]
want = [c for c in ("id","kind","service_id","status","request_id","created_at","started_at") if c in cols]
for r in db.execute(f"select {','.join(want)} from service_operations order by rowid"): print(dict(zip(want, r)))
