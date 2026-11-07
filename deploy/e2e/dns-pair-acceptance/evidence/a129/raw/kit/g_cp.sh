python3 - <<"P"
import sqlite3
c=sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro",uri=True)
print(c.execute("select id,name,domain_id,created_at from databases_v2").fetchall())
for t in ("audit_logs","audit_log"):
  try: print(c.execute("select * from %s where action like \"%%copy%%\" or action like \"%%database%%\" order by id desc limit 8"%t).fetchall())
  except Exception as e: print(t,e)
P
mariadb -N -e "select user,host from mysql.user where user like \"tgt%\""; mariadb -N -e "show databases like \"tgt%\""; ls -la /var/backups/celikpanel/ | head; journalctl -u celikpanel-agent -u celikpanel-panel -o short-iso --no-pager | grep -iE "copy|91f3|c4b5" | cut -c1-300 | tail -12
