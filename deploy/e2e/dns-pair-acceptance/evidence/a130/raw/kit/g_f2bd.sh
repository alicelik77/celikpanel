pkill -f p129e.sh; pkill -f k4remote.sh; sleep 1
echo "--- log_warnings: $(mariadb -N -e "select @@log_warnings, @@version")"
echo "--- jail:"; cat /etc/fail2ban/jail.d/celikpanel-database-remote.conf 2>&1 | head -20; ls /etc/fail2ban/filter.d/ | grep -i celik; cat /etc/fail2ban/filter.d/celikpanel*.conf 2>/dev/null | grep -vE "^#|^$" | head -15
echo "--- mariadb journal (access denied):"; journalctl -u mariadb --since -15min -o short-iso --no-pager | grep -iE "denied|access" | tail -5 | cut -c1-250; journalctl -u mariadb --since -15min -o cat --no-pager | tail -3 | cut -c1-200
fail2ban-client get celikpanel-mariadb-auth journalmatch 2>&1 | head -3
