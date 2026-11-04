journalctl -u celikpanel-panel -u celikpanel-agent --no-pager --since "16:40:50" --until "16:41:30" 2>/dev/null | grep -viE "certificate startup" | tail -25 | cut -c1-350
