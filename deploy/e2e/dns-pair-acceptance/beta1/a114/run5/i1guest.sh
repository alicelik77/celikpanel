P=$(journalctl -b all -u celikpanel-panel --no-pager -o cat 2>/dev/null || journalctl -u celikpanel-panel --no-pager -o cat)
A=$(journalctl -b all -u celikpanel-agent --no-pager -o cat 2>/dev/null || journalctl -u celikpanel-agent --no-pager -o cat)
grep PRETTY_NAME /etc/os-release
echo "panel journal lines: $(echo "$P" | wc -l); agent journal lines: $(echo "$A" | wc -l)"
echo "firewall-waits (panel): $(echo "$P" | grep -c 'the firewall update waits')"
echo "firewall_sync_failed (panel): $(echo "$P" | grep -c firewall_sync_failed)"
echo "firewall_sync_host_busy (panel): $(echo "$P" | grep -c firewall_sync_host_busy)"
echo "fail-closed: agent=$(echo "$A" | grep -ci fail-closed) panel=$(echo "$P" | grep -ci fail-closed)"
echo "could not be checked: agent=$(echo "$A" | grep -ci 'could not be checked') panel=$(echo "$P" | grep -ci 'could not be checked')"
echo "-- firewall / busy lines (panel)"
echo "$P" | grep -E 'firewall update waits|/firewall|another server change|firewall_waiting' | cut -c1-300 | tail -10
echo "-- setup step lines"
echo "$P" | grep -E 'server setup step' | cut -c1-200 | tail -6
