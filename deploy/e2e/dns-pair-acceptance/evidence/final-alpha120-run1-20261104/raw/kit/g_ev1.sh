# guest (root, read only): evidence for the stuck 01-dns step. Lab only.
date -u
cat /opt/celikpanel/release.commit 2>/dev/null
/usr/libexec/celikpanel/recovery dns-switch-status --quiesced --request-id $1 2>&1 | head -10
python3 - <<'P'
import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
for t in ("dns_engine_switch_snapshots","dns_engine_state","service_operations"):
    try:
        cols=[r[1] for r in db.execute("pragma table_info(%s)" % t)]; print("##", t, cols)
        for r in db.execute("select * from %s order by rowid desc limit 5" % t): print("  ", str(r)[:700])
    except Exception as e: print(t, e)
P
journalctl -u celikpanel-agent -u celikpanel-panel --no-pager 2>/dev/null | grep -v "certificate startup reconcile" | tail -40 | cut -c1-300
systemctl is-active pdns celikpanel-agent celikpanel-panel nginx
