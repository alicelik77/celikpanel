# guest (root, read-only except the hand-started second Agent the owner might run): case 1 checks. Lab only.
echo "release $(cat /opt/celikpanel/release.commit 2>/dev/null)"
AG=$(systemctl show -p ExecStart --value celikpanel-agent | sed -n 's/.*path=\([^ ;]*\).*/\1/p'); echo "agent binary $AG"
stat -c '%A %U:%G inode=%i %n' /run/celikpanel /run/celikpanel/agent.sock 2>&1
if [ "${1:-}" = full ]; then
  echo "--- $AG --version"; timeout 10 $AG --version; echo "rc=$?"
  echo "--- $AG bogus"; timeout 10 $AG bogus 2>&1 | head -3; echo "rc=${PIPESTATUS[0]}"
  PID0=$(systemctl show -p MainPID --value celikpanel-agent)
  echo "--- a second Agent started by hand (as root, outside systemd)"; timeout 20 $AG 2>&1 | head -6; echo "rc=${PIPESTATUS[0]}"
  stat -c '%A %U:%G inode=%i %n' /run/celikpanel/agent.sock; echo "systemd agent pid $PID0 -> $(systemctl show -p MainPID --value celikpanel-agent) active=$(systemctl is-active celikpanel-agent)"
  echo "agent processes: $(pgrep -fc "^$AG")"
fi
echo "--- failed self-update units"; systemctl list-units --failed --all --no-legend --plain 'celikpanel-self-update-*' | cat
echo "--- all self-update units"; systemctl list-units --all --no-legend --plain 'celikpanel-self-update-*' | cat
echo "--- attempt records"; ls -la /var/lib/celikpanel-release-state/self-update/ 2>&1 | tail -n +2
for f in /var/lib/celikpanel-release-state/self-update/*/state.json /var/lib/celikpanel-release-state/self-update/*.json; do [ -f "$f" ] && python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(" ", sys.argv[1].split("/")[-2:], {k:d.get(k) for k in ("request_id","status","phase","created_at","from_version","target_version","reason","code")})' "$f" 2>/dev/null; done
echo "--- agent journal (self-update / reset / instance)"; journalctl -u celikpanel-agent --since -40min -o cat --no-pager | grep -iE "self-update|reset-failed|instance lock|failed unit" | tail -12
