# guest (root): alpha.130 remote-access probes on A. usage: g_k2.sh skip|pg|f2b|files|nft|unban IP|expire RULEID|ruletab|hang|unhang|state. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
DB=/var/lib/celikpanel/celikpanel.db
case "$1" in
skip) mariadb -N -B -e "select concat('skip_name_resolve=',@@skip_name_resolve)" 2>&1; ls -la /etc/mysql/mariadb.conf.d/ 2>&1 | grep -i celik; grep -rn "skip.name.resolve" /etc/mysql 2>/dev/null | head
      mariadb -N -B -e "select user,host from mysql.user where host not in ('localhost','127.0.0.1','::1')" 2>&1 | head ;;
pg) P=$(pg_lsclusters -h | awk '$4 ~ /online/ {print $3; exit}'); echo "port $P"; sudo -u postgres psql -X -A -t -p $P -c "show log_line_prefix" 2>&1
    grep -rn "celikpanel-client\|log_line_prefix" /etc/postgresql/*/main/conf.d/ 2>/dev/null | head; grep -n "192.0.3" /etc/postgresql/*/main/pg_hba.conf 2>/dev/null | head ;;
f2b) J=$(fail2ban-client status 2>/dev/null | sed -n 's/.*Jail list:\s*//p' | tr ',' ' '); echo "jails: $J"
     for j in $J; do fail2ban-client status $j 2>/dev/null | grep -E "Currently failed|Total failed|Currently banned|Banned IP" | sed "s/^/  $j: /"; done ;;
files) ls -la /etc/fail2ban/jail.d /etc/fail2ban/filter.d 2>/dev/null | grep -i celik; ls /etc/postgresql/*/main/conf.d/ 2>/dev/null ;;
nft) nft list table inet celikpanel_dbremote 2>&1 | grep -E "dport|saddr|elements|192.0.3" | head -8; echo "tables: $(nft list tables 2>&1 | xargs)" ;;
unban) for j in $(fail2ban-client status 2>/dev/null | sed -n 's/.*Jail list:\s*//p' | tr ',' ' '); do fail2ban-client set $j unbanip $2 >/dev/null 2>&1; done; echo "unbanned $2" ;;
expire) python3 -c '
import sqlite3, sys
c = sqlite3.connect(sys.argv[1], timeout=20)
c.execute("UPDATE database_remote_rules SET expires_at = strftime(\"%Y-%m-%d %H:%M:%S\",\"now\",\"-1 minute\") WHERE id = ?", (int(sys.argv[2]),))
c.commit(); print("rule", sys.argv[2], "expires_at", c.execute("select expires_at from database_remote_rules where id=?", (int(sys.argv[2]),)).fetchone())' "$DB" "$2" ;;
ruletab) python3 -c '
import sqlite3, sys
c = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
for r in c.execute("select id, database_id, address, created_at, expires_at from database_remote_rules"): print("  rule", r)
for r in c.execute("select created_at, action, substr(coalesce(details,\"\"),1,160) from audit_logs where action like \"database.remote%\" order by id desc limit 8"): print("  audit", r)' "$DB" ;;
hang) printf '[Unit]\nDescription=lab: holds the boot in starting for 5 minutes (lab only)\n[Service]\nType=oneshot\nExecStart=/bin/sleep 300\nTimeoutStartSec=infinity\n[Install]\nWantedBy=multi-user.target\n' > /etc/systemd/system/lab-hang.service
      systemctl daemon-reload; systemctl enable lab-hang.service 2>&1 | tail -1 ;;
unhang) systemctl disable lab-hang.service 2>/dev/null; rm -f /etc/systemd/system/lab-hang.service; systemctl daemon-reload; echo unhung ;;
state) echo "is-system-running: $(systemctl is-system-running 2>&1)"; nft list table inet celikpanel_dbremote 2>&1 | grep -E "192.0.3" | head -4 ;;
esac
