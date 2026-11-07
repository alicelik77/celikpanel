# guest (root, read-only): the panel database's applied migrations (sqlite read-only). Lab only.
F=$(grep -hE '^CELIKPANEL_DB(_PATH)?=' /etc/celikpanel/panel.env 2>/dev/null | head -1 | cut -d= -f2)
[ -f "$F" ] || F=$(find /var/lib/celikpanel -maxdepth 2 -name '*.db' -size +10k 2>/dev/null | head -1)
python3 - "$F" <<'P'
import sqlite3, sys
c = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
rows = c.execute("select version, filename from schema_migrations order by version").fetchall()
print("db", sys.argv[1], "migrations", len(rows), "last:", rows[-6:])
for t in ("setting_policy", "mail_outgoing_settings", "site_web_settings", "api_tokens", "site_visitor_stats_daily"):
    try: print("  ", t, c.execute("select count(*) from %s" % t).fetchone()[0], "rows")
    except Exception as e: print("  ", t, "ERR", e)
try: print("   remote api / outgoing enabled:", c.execute("select enabled from mail_outgoing_settings").fetchall())
except Exception as e: print("  ", e)
P
