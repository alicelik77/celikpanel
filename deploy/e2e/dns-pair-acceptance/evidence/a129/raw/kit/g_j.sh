P=$(echo "${3:-}" | base64 -d 2>/dev/null); P=${P:-copy|database|error|internal|create}
journalctl --since "$1" --until "$2" -o cat --no-pager -u celikpanel-panel -u celikpanel-agent | grep -iE "$P" | cut -c1-500 | tail -${4:-15}
