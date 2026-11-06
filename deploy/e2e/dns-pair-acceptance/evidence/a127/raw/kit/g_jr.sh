# guest (root, read-only): panel + agent journal lines between $1 and $2 matching $3. Lab only.
journalctl -u celikpanel-panel -u celikpanel-agent --since "$1" --until "$2" -o short-iso --no-pager | grep -iE "${3:-.}" | cut -c1-600 | tail -${4:-40}
