U=$(systemctl list-units --all --no-legend --plain "celikpanel-self-update-*" | awk "{print \$1}" | head -1); echo unit $U
journalctl -u $U -o short-iso --no-pager | cut -c1-400 | tail -60
echo ---- frozen grep
journalctl --since "2026-11-07 14:59:50" --until "2026-11-07 15:01:20" -o short-iso --no-pager | grep -iE "froz|freeze|thaw|celikpanel-panel|recovery|update" | cut -c1-400 | head -60
