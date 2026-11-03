grep -rn "file_uploads" /etc/php/*/fpm/pool.d/ 2>/dev/null; ls -la --time-style=+%T /etc/php/8.4/fpm/pool.d/ | grep -E "site2|c118" 
python3 -c 'import sqlite3; db=sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro",uri=True); print([r for r in db.execute("select * from subscription_disk_usage")])' 2>&1 | cut -c1-600
journalctl -u celikpanel-panel --no-pager --since -25min 2>/dev/null | grep -iE "\[disk\]|upload" | tail -8 | cut -c1-300
