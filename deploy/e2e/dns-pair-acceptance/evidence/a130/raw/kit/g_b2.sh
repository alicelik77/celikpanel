ls -la /etc/postgresql-common 2>&1; dpkg -S /etc/postgresql-common/createcluster.conf /etc/postgresql-common/root.crt 2>&1 | head -3
ls -la /var/backups/celikpanel/kept 2>/dev/null | head
echo "-- dovecot"; dpkg -l 'dovecot*' 2>/dev/null | grep -E '^(ii|rc|iU|iF)'; ls -la /usr/sbin/dovecot 2>&1; command -v doveconf; ls /etc/dovecot 2>&1 | head
echo "-- postfix"; dpkg -l postfix 2>/dev/null | tail -1 | cut -c1-70; systemctl is-active postfix postfix@- 2>&1 | tr "\n" " "; ls /etc/systemd/system/postfix.service.d 2>&1
journalctl -u celikpanel-agent --since 12:30:00 -o cat --no-pager | grep -iE "dovecot|version" | tail -5 | cut -c1-250
