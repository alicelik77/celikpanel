find /var/backups/celikpanel/kept -type f -printf '%M %u:%g %p\n'; ls -ld /etc/systemd/system/postfix.service.d
journalctl -u celikpanel-panel -u celikpanel-agent --since 13:24:30 -o cat --no-pager | grep -iE "kept|owner file|put back" | tail -4 | cut -c1-300
