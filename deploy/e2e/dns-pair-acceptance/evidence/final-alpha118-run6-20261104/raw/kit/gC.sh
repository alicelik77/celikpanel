echo "## $(date -u +%T) $(grep PRETTY /etc/os-release)"; nginx -t 2>&1 | tail -1
cat /etc/nginx/celikpanel-managed.d/ca118.test/access-log.traffic 2>&1 | grep -v '^#'; ls -la /var/log/celikpanel/traffic/ 2>&1 | grep -v total
echo "## dovecot $(dovecot --version)"; doveconf -n >/dev/null 2>/tmp/dc.err; echo "doveconf rc=$? $(head -2 /tmp/dc.err)"
ls /etc/dovecot/conf.d/ 2>&1 | tr '\n' ' '; echo; cat /etc/dovecot/conf.d/90-celikpanel-quota.conf 2>&1 | grep -v '^#' | head -30
postconf -h smtpd_recipient_restrictions; command -v logrotate || echo "no logrotate"
