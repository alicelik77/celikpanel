echo "--- installed guard script"; grep -n 'cgroup" \] || continue\|render' /usr/lib/celikpanel-sandbox/egress-guard.sh | head -3
echo "--- guard runs in the last 60 s"; journalctl -u celikpanel-egress-guard.service --since -60s -o short-iso --no-pager | grep -E "Finished|Failed with|not blocked" | tail -6 | cut -c1-200
echo "--- the finished step"; systemctl list-units --all --no-legend --plain 'celikappinst-*' | cat
echo "--- egress set"; nft list set inet celikpanel_egress smtp_blocked 2>/dev/null | grep -o 'system.slice/[a-z0-9-]*\.service' | tr '\n' ' '; echo
echo "--- systemctl --failed"; systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"
