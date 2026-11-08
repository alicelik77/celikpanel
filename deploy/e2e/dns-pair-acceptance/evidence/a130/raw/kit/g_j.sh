journalctl -u celikpanel-panel -u celikpanel-agent --since "$1" --until "$2" -o short-iso --no-pager | grep -viE "GET /api|health" | cut -c1-330 | tail -${3:-40}
