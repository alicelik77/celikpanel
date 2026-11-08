# guest (root, read-only): what a removal left. usage: g_rt.sh all|pg|postfix|dovecot. Lab only.
pkgs() { dpkg -l 2>/dev/null | grep -E '^(ii|rc|iU|iF)' | awk '{print $1, $2}' | grep -E "$1" | xargs; }
case "$1" in
pg|all) echo "pg dpkg: $(pkgs 'postgres|libpq')"; ls -d /etc/postgresql /etc/postgresql-common /var/lib/postgresql /var/log/postgresql 2>&1 | xargs
        ls /etc/apt/sources.list.d /etc/apt/preferences.d /etc/apt/keyrings 2>/dev/null | grep -iE "pgdg|postgres" | xargs ;;&
postfix|all) echo "postfix dpkg: $(pkgs 'postfix')"; ls -la /etc/postfix 2>&1 | tail -n +2 | awk '{print $1, $3":"$4, $NF}' | xargs -L1 echo "  " | head -20
        ls -la /etc/systemd/system/postfix.service.d 2>&1 | tail -n +2 | head -5; systemctl is-active postfix postfix@- 2>&1 | xargs ;;&
dovecot|all) echo "dovecot dpkg: $(pkgs 'dovecot')"; find /etc/dovecot 2>&1 | head -10 ;;&
*) echo "kept:"; find /var/backups/celikpanel/kept -maxdepth 3 2>/dev/null | head -30
   systemctl list-timers --all --no-legend 2>/dev/null | grep -iE "celikpanel|postfix|dovecot" | awk '{print $(NF-1), $NF}' | xargs ;;
esac
