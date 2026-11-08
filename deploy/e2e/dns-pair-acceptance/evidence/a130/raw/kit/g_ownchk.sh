ls -la /etc/systemd/system/postfix.service.d/ 2>&1; cat /etc/systemd/system/postfix.service.d/owner.conf 2>&1; dpkg -l | grep -E "^rc" | awk '{print $1,$2}'; ls -la /etc/dovecot /etc/dovecot/conf.d
journalctl -u celikpanel-agent --since "2026-11-08 10:43:55" -o cat --no-pager | grep -iE "owner|kept|systemd/system/postfix" | head -8 | cut -c1-300
