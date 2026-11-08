echo "first guard failure: $(journalctl -u celikpanel-egress-guard.service -o short-iso --no-pager | grep -m1 'Failed with result' | cut -c1-25)"
echo "failures so far: $(journalctl -u celikpanel-egress-guard.service -o cat --no-pager | grep -c 'Failed with result')"
journalctl -u celikpanel-egress-guard.service -o short-iso --no-pager | grep -m3 "not blocked" | cut -c1-200
systemctl list-units --all --no-legend --plain 'celikappinst-*' 'celiksvcinst-*' | cat
systemctl show -P ControlGroup celikappinst-3.service; echo "(cgroup end)"
nft list set inet celikpanel_egress smtp_blocked 2>/dev/null | grep -o 'system.slice/[a-z0-9-]*\.service' | tr '\n' ' '; echo
