# guest (root, read-only): CelikPanel's mail files, units and timers; leftover packages. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
echo "-- timers"; systemctl list-timers --all --no-legend --plain 2>/dev/null | grep -iE "celikpanel|quota|postfix|dovecot" | awk '{print $(NF-1), $NF}'
echo "-- unit files"; systemctl list-unit-files --no-legend --plain 2>/dev/null | grep -iE "celikpanel-quota|postfix|dovecot|postfwd" | cat
echo "-- celikpanel files"; ls -la /etc/systemd/system 2>/dev/null | grep -iE "quota|postfix|dovecot" ; find /etc/postfix /etc/dovecot /etc/systemd/system/postfix* /etc/systemd/system/dovecot* /usr/local/libexec /var/lib/celikpanel* -maxdepth 3 \( -iname "*celikpanel*" -o -iname "*quota*" -o -name "owner.conf" \) 2>/dev/null | sort
echo "-- dirs"; ls -d /etc/postfix /etc/dovecot /etc/dovecot/conf.d 2>&1 | tr '\n' ' '; echo
echo "-- packages"; dpkg -l 2>/dev/null | grep -E "postfix|dovecot|postfwd" | awk '{print $1, $2}'
