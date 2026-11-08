# guest (root, read-only): case 1 checks after an update. $1 = since (e.g. -30min)
S=${1:--40min}
echo "release $(cat /opt/celikpanel/release.commit 2>/dev/null)"
echo "--- systemctl --failed (all units)"; systemctl list-units --failed --all --no-legend --plain | cat; echo "(end)"
echo "--- self-update units"; systemctl list-units --all --no-legend --plain 'celikpanel-self-update-*' | cat
echo "--- 'database publication is unverified' in the journal since $S: $(journalctl --since "$S" -o cat --no-pager | grep -c 'database publication is unverified')"
echo "--- late database check lines (reason / re-read)"
journalctl --since "$S" -o cat --no-pager | grep -E "Recovery database check|database changed while|reading it once more|concurrent.write|verify-update-database" | cut -c1-260 | tail -12
echo "--- waits (nothing needs to be done)"
journalctl --since "$S" -o cat --no-pager | grep -E "nothing needs to be done|waiting up to" | cut -c1-260 | tail -8
echo "--- agent journal (self-update / cleared)"
journalctl -u celikpanel-agent --since "$S" -o cat --no-pager | grep -iE "self-update|reset-failed|cleared|recovery" | cut -c1-240 | tail -10
echo "--- attempt records"
for f in /var/lib/celikpanel-release-state/self-update/*/state.json /var/lib/celikpanel-release-state/self-update/*.json; do [ -f "$f" ] && python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(" ", sys.argv[1].split("/")[-2:], {k:d.get(k) for k in ("request_id","status","phase","created_at","from_version","target_version","reason","code")})' "$f" 2>/dev/null; done
