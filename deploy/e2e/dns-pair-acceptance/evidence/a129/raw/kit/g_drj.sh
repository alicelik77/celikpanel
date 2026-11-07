# guest (root, read-only): panel and agent journal lines about remote database access since $1. Lab only.
journalctl -u celikpanel-panel -u celikpanel-agent --since "$1" --until "${2:-now}" -o short-iso --no-pager | grep -iE "database.remote|database_remote|dbremote|plan|rights" | cut -c1-300 | tail -40
