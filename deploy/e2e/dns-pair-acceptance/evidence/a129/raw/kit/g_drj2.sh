# guest (root, read-only). Lab only.
journalctl -u celikpanel-panel -u celikpanel-agent --since "20:12:30" --until "20:15:00" -o short-iso --no-pager | grep -iE "remote|apply|firewall|refus|busy|error" | cut -c1-350 | tail -15
systemctl is-system-running; nft list tables | xargs
