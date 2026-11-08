# guest (root, read-only): why PostgreSQL does not start (Arch). Lab only.
ls -ld /etc/ssl/celikpanel-database /etc/ssl/celikpanel-database/* 2>&1 | head -5
systemctl is-active postgresql; journalctl -u postgresql -b --no-pager -o short-iso | tail -6 | cut -c1-250
journalctl -u celikpanel-agent --since "19:30" --no-pager -o cat | grep -iE "remote|certif|postgres" | tail -6 | cut -c1-300
