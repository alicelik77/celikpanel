journalctl -u celikrt-2.service --no-pager -o short-precise 2>/dev/null | grep -E "SANDBOX-PROBE (4|5|6) |SANDBOX-PROBE START|panic|signal|SIGSYS|exit" | head -20 | cut -c1-300
journalctl -u celikrt-2.service --no-pager 2>/dev/null | grep -c "SANDBOX-PROBE"
