find /etc/dovecot -maxdepth 2 -printf '%M %u:%g %s %TY-%Tm-%Td %TH:%TM %p\n' 2>&1 | head; echo "users lines: $(wc -l < /etc/dovecot/users 2>/dev/null)"
pgrep -a apt; pgrep -a dpkg; ls /var/lib/dpkg/lock* ; fuser /var/lib/dpkg/lock-frontend 2>&1
journalctl -u celikpanel-agent --since 12:29:30 -o short-iso --no-pager | grep -iE "dovecot|postfix|package|busy" | tail -12 | cut -c1-260
