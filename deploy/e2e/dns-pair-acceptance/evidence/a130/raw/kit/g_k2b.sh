cat /etc/mysql/mariadb.conf.d/90-celikpanel-remote-access.cnf
journalctl -u mariadb --since "2026-11-08 11:00" -o short-iso --no-pager | grep -iE "starting|shutdown|ready for" | tail -6 | cut -c1-200
python3 -c '
import sqlite3
c=sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro",uri=True)
for r in c.execute("select key,value from panel_settings where key like \"%remote%\""): print(r)
for r in c.execute("select created_at, action, substr(coalesce(details,\"\"),1,200) from audit_logs where action like \"database.remote%\" or action like \"service.%\" order by id desc limit 10"): print(r)'
