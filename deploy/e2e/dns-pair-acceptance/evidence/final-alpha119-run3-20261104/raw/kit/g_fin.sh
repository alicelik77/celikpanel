echo "## panics"; journalctl -u celikpanel-panel -u celikpanel-agent --no-pager 2>/dev/null | grep -ciE 'panic:|goroutine [0-9]+ \['
systemctl --failed --no-legend --no-pager | head
