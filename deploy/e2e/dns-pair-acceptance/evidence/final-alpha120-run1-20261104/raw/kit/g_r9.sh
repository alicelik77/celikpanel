# guest (root, read only): after the 117 -> 119 update. Lab only.
echo "## $(date -u +%T) $(grep PRETTY /etc/os-release)"; ls /opt/celikpanel
python3 - <<'P'
import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
print("migrations >=56:", [r[0] for r in db.execute("select version from schema_migrations where version>=56 order by version")])
for t in ("usage_notices","rights_notices"):
    try: print(t, db.execute("select * from %s" % t).fetchall()[:5])
    except Exception as e: print(t, e)
print("subscriptions bandwidth_quota_mb:", db.execute("select id, bandwidth_quota_mb from subscriptions").fetchall())
P
echo "## services"; systemctl is-active celikpanel-panel celikpanel-agent nginx postfix dovecot mariadb pdns named 2>&1 | tr '\n' ' '; echo; systemctl --failed --no-legend --no-pager | head -5
echo "## nginx -t"; nginx -t 2>&1 | tail -1
echo "## panics: $(journalctl -u celikpanel-panel -u celikpanel-agent --no-pager 2>/dev/null | grep -ciE 'panic:|goroutine [0-9]+ \[')"
echo "## env migration (118) at agent start"; journalctl -u celikpanel-agent --no-pager --since -15min 2>/dev/null | grep -iE "environment|svcapp|service app|env" | tail -5 | cut -c1-250
ls -la /etc/celikpanel-svcapps 2>&1 | head -3
journalctl -u celikpanel-panel -u celikpanel-agent -p err --no-pager --since -15min 2>/dev/null | tail -6 | cut -c1-250
