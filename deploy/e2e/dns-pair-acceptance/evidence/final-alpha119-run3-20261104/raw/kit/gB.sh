# guest (root, read only): version, migrations, services, traffic/quota config. Lab only.
echo "## $(date -u +%T) release"; cat /opt/celikpanel/release.commit 2>/dev/null; ls -la /opt/celikpanel/ | head -5
python3 - <<'P'
import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
print("schema versions >=56:", db.execute("select version from schema_migrations where version>=56 order by version").fetchall())
P
echo "## services"; systemctl is-active celikpanel-panel celikpanel-agent nginx postfix dovecot named bind9 mariadb 2>&1 | tr '\n' ' '; echo
systemctl --failed --no-legend --no-pager | head -5
echo "## nginx -t"; nginx -t 2>&1 | tail -1
echo "## traffic"; ls /etc/nginx/conf.d/ | grep -i traffic; ls -la /var/log/celikpanel/traffic/ 2>&1 | grep -v total
echo "## dovecot $(dovecot --version)"; head -30 /etc/dovecot/conf.d/90-celikpanel-quota.conf 2>&1; doveconf -n >/dev/null 2>/tmp/dc.err; echo "doveconf rc=$? $(head -2 /tmp/dc.err)"
postconf -h smtpd_recipient_restrictions
doveadm quota get -u q@cb117.test 2>&1 | head -3
grep -iE "552|5\.2\.2" /var/log/mail.log 2>/dev/null | tail -3
echo "## panics"; journalctl -u celikpanel-panel -u celikpanel-agent --no-pager 2>/dev/null | grep -ciE 'panic:|goroutine [0-9]+ \['
journalctl -u celikpanel-panel -u celikpanel-agent -p err --no-pager --since -30min 2>/dev/null | tail -5 | cut -c1-250
