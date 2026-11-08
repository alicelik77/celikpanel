echo "is-system-running: $(systemctl is-system-running)"; cat /etc/mysql/mariadb.conf.d/90-celikpanel-remote-access.cnf | grep -v "^#"
mariadb -N -B -e "select concat('skip_name_resolve=',@@skip_name_resolve)"
python3 -c '
import sqlite3
c=sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro",uri=True)
for r in c.execute("select key,value,updated_at from panel_settings where key like \"%remote%\""): print(r)
for r in c.execute("select created_at, action, substr(coalesce(details,\"\"),1,200) from audit_logs where action like \"database.remote%\" order by id desc limit 12"): print(r)'
journalctl -u celikpanel-panel -u celikpanel-agent --since "2026-11-08 11:20" -o short-iso --no-pager | grep -iE "name.resolve|name lookup|skip" | tail -8 | cut -c1-250
