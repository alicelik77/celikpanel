# guest (root, read-only): the recovery watchdog's runs since $1 (CPU per run from systemd), its runner, its stamp. Lab only.
S=${1:--30min}
U=celikpanel-release-recovery.service
X=$(systemctl show -p ExecStart --value $U | grep -o 'path=[^ ;]*' | head -1 | cut -d= -f2)
echo "unit ExecStart path: $X"; systemctl show -p ExecStart --value $U | cut -c1-200
for f in $X /usr/libexec/celikpanel/release-recovery-runner /usr/local/libexec/celikpanel/release-recovery-runner; do [ -f "$f" ] && echo "runner $f sha $(sha256sum $f | cut -c1-16) fast-path-function: $(grep -c release_recovery_idle_fast_path $f)"; done
echo "timer: $(systemctl show -p ActiveState --value celikpanel-release-recovery.timer) next $(systemctl show -p NextElapseUSecRealtime --value celikpanel-release-recovery.timer)"
echo "stamp: $(cat /run/celikpanel-release-recovery/idle-verified 2>&1)"
journalctl -u $U --since "$S" -o short-iso --no-pager | grep -oE '^[^ ]+|Consumed [0-9.]+m?s CPU time' | paste - - 2>/dev/null | grep Consumed > /tmp/wd.txt
journalctl -u $U --since "$S" -o short-iso --no-pager | grep -E "Consumed" | sed -E 's/^([^ ]+) .*Consumed ([0-9.]+)(m?s) CPU time.*/\1 \2\3/' > /tmp/wd.txt
echo "runs with a CPU line since $S: $(wc -l < /tmp/wd.txt)"; tail -${2:-12} /tmp/wd.txt
python3 - /tmp/wd.txt <<'P'
import sys, re
v = []
for line in open(sys.argv[1]):
    m = re.search(r' ([0-9.]+)(ms|s)$', line.strip())
    if m: v.append(float(m.group(1)) * (1 if m.group(2) == "ms" else 1000))
if v:
    s = sorted(v); print("CPU ms per run: n=%d min %.0f median %.0f max %.0f; runs over 100 ms: %d" % (len(v), s[0], s[len(s)//2], s[-1], sum(1 for x in v if x > 100)))
P
echo "--- other runner lines since $S"; journalctl -u $U --since "$S" -o short-iso --no-pager | grep -vE "Consumed|Starting|Finished|Deactivated|start request repeated" | cut -c1-300 | tail -${3:-15}
