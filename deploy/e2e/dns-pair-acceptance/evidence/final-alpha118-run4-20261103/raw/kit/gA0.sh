# guest (root, read only): traffic/nginx/dovecot views. usage: gA0.sh DOMAINID. Lab only.
echo "## nginx -t"; nginx -t 2>&1 | tail -2
echo "## traffic conf"; cat /etc/nginx/conf.d/celikpanel-traffic.conf 2>&1 | head -12
echo "## includes"; ls -la /etc/nginx/celikpanel-managed.d/*/ 2>&1 | head -20; cat /etc/nginx/celikpanel-managed.d/*/access-log.traffic 2>&1 | head -5
echo "## traffic logs"; ls -la /var/log/celikpanel/traffic/ 2>&1
echo "## logrotate"; cat /etc/logrotate.d/celikpanel-traffic 2>&1 | head -12
echo "## dovecot"; dovecot --version; ls -la /etc/dovecot/conf.d/ | grep -i celik; cat /etc/dovecot/conf.d/90-celikpanel-quota.conf 2>&1 | head -40
echo "## doveconf -n rc"; doveconf -n >/dev/null 2>/tmp/dc.err; echo "rc=$? $(head -3 /tmp/dc.err)"
echo "## postfix restrictions"; postconf -h smtpd_recipient_restrictions; postconf -M | grep -i quota
echo "## quota warning"; ls -la /usr/local/libexec/celikpanel-quota-warning /etc/systemd/system/celikpanel-quota-warning.* 2>&1; systemctl list-timers celikpanel-quota-warning.timer --no-pager 2>&1 | head -3
echo "## users quota lines"; grep "@c118.test" /etc/dovecot/users 2>/dev/null | while IFS= read -r l; do echo "${l%%:*} $(echo "$l" | grep -oE 'userdb_quota[a-z_]*=[^ :]*' | tr '\n' ' ')"; done
echo "## site docroot"; grep -rh "root " /etc/nginx/sites-enabled/*c118* 2>/dev/null | head -3
