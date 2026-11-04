python3 - <<'P'
import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
ts=[r[0] for r in db.execute("select name from sqlite_master where type='table'") if 'audit' in r[0] or 'log' in r[0] or 'mutation' in r[0]]
print(ts)
for t in ts:
    cols=[r[1] for r in db.execute("pragma table_info(%s)" % t)]
    print("##", t, cols)
    try:
        for r in db.execute("select * from %s where rowid > (select max(rowid)-12 from %s)" % (t,t)): print("  ", str(r)[:400])
    except Exception as e: print(e)
P
ls -la /var/lib/celikpanel-agent* /var/lib/celikpanel/agent* /var/lib/celikpanel-recovery 2>/dev/null | head; find /var/lib -maxdepth 2 -iname '*ledger*' -o -maxdepth 2 -iname '*mutation*' 2>/dev/null | head
