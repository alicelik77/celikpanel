# guest (root, read only): mailbox quota warning state for MAILBOX. Lab only.
M=$1
echo "## $(date -u +%T) $(dovecot --version)"; systemctl list-timers celikpanel-quota-warning.timer --no-pager --all 2>&1 | head -3
journalctl -u celikpanel-quota-warning.service --no-pager 2>/dev/null | tail -6 | cut -c1-250
ls -la /var/lib/celikpanel-quota-warning/ 2>&1 | head
doveadm quota get -u $M 2>&1 | head -3
echo "## messages in $M (subjects)"; doveadm fetch -u $M 'hdr.subject date.received' mailbox INBOX ALL 2>&1 | grep -iE '^hdr.subject|^date' | head -10
echo "## postfix quota rejects"; journalctl -u postfix@- -u postfix --no-pager --since -90min 2>/dev/null | grep -E '552 5\.2\.2' | tail -3 | cut -c1-250; grep -E '552 5\.2\.2' /var/log/mail.log 2>/dev/null | tail -2 | cut -c1-250
echo "## quota conf head"; sed -n 1,3p /etc/dovecot/conf.d/90-celikpanel-quota.conf 2>&1; postconf -h smtpd_recipient_restrictions
