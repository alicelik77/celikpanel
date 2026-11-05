# guest (root, read-only): panel journal around the restart during remove-together. Lab only.
journalctl -u celikpanel-panel --since "05:11:40" --until "05:13:05" -o short-precise --no-pager | grep -vE "GET /api" | cut -c1-260 | grep -iE "stop|start|remov|operation|resum|shutdown|wait|drain|signal|interrupt|lease|phppgadmin|postgres" | head -30
journalctl -u celikpanel-agent --since "05:11:40" --until "05:13:05" -o short-precise --no-pager | cut -c1-220 | grep -iE "remov|phppgadmin|postgres|lease|mutation" | head -12
