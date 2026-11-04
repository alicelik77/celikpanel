date -u +%T; journalctl -u celikpanel-agent -u celikpanel-panel --no-pager --since "13:59:45" 2>/dev/null | grep -v "certificate startup reconcile" | tail -40 | cut -c1-260
ps -eo pid,etime,cmd | grep -E 'apt|dpkg|pdns' | grep -v grep | head
