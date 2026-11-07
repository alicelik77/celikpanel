systemctl status celikpanel-egress-guard.service --no-pager 2>&1 | head -20 | cut -c1-300
echo "---"; journalctl -u celikpanel-egress-guard.service -o short-iso --no-pager | tail -25 | cut -c1-300
echo "---"; systemctl cat celikpanel-egress-guard.service 2>&1 | head -40
systemctl list-timers --all --no-pager 2>/dev/null | grep -i egress
