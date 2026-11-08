# guest (root, read-only): the MariaDB jail's filter and its matches on the journal. Lab only.
ls -la /etc/fail2ban/filter.d/ | grep -i celik; ls /etc/fail2ban/jail.d/ | grep -i celik
grep -n "failregex\|journalmatch\|^\[" /etc/fail2ban/filter.d/celikpanel-mariadb-auth.conf 2>/dev/null | cut -c1-400
cat /etc/fail2ban/jail.d/celikpanel-mariadb*.conf 2>/dev/null | grep -v "^#" | head -20
journalctl --since "19:36:00" --until "19:37:10" -o short-iso --no-pager | grep -i "access denied" | head -3 | cut -c1-250
fail2ban-client get celikpanel-mariadb-auth failregex 2>&1 | cut -c1-400
fail2ban-client get celikpanel-mariadb-auth journalmatch 2>&1 | cut -c1-200
