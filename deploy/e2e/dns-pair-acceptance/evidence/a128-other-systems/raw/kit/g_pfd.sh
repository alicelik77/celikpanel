journalctl -u postfwd --since "2026-11-07 15:41:50" -o short-iso --no-pager | grep -viE "\[RULES\]|STATS" | cut -c1-260 | head -40
systemctl cat postfwd | grep -E "ExecReload|ExecStart=/" 
journalctl -u celikpanel-agent --since "2026-11-07 15:41:50" -o cat --no-pager | grep -iE "postfwd|outgoing" | cut -c1-300 | head
journalctl -u postfwd --since "2026-11-07 15:42:10" -o short-iso --no-pager | grep -E "box1" | head -3 | cut -c1-400
