# guest (root, read-only): the refused 126 -> 127 attempt (13:28:47) and its rollback; unit states in between. Lab only.
echo "--- release-recovery runner lines 13:28:45-13:29:48"
journalctl -u celikpanel-release-recovery.service --since 13:28:45 --until 13:29:48 -o short-iso --no-pager | cut -c1-260 | grep -v "Created symlink"
echo "--- systemd unit results 13:28:45-13:29:48 (failed / succeeded / deactivated)"
journalctl --since 13:28:45 --until 13:29:48 -o short-iso --no-pager _PID=1 | grep -E "celikpanel" | grep -iE "fail|result|Deactivated|Finished|Started" | cut -c1-220 | tail -25
echo "--- update failure line"; journalctl --since 13:28:45 --until 13:29:48 -o cat --no-pager | grep -E "CELIKPANEL_UPDATE_FAILURE|rolled back|recovery completed|restored" | cut -c1-300 | tail -8
echo "--- per-run CPU of the watchdog (systemd CPU_USAGE_NSEC on each stop) since 13:31"
journalctl -u celikpanel-release-recovery.service --since 13:31:00 -o json --no-pager | python3 -c '
import json,sys
v=[]
for l in sys.stdin:
    d=json.loads(l)
    if "CPU_USAGE_NSEC" in d: v.append(int(d["CPU_USAGE_NSEC"])/1e6)
print("runs with CPU_USAGE_NSEC:", len(v), "ms:", [round(x,1) for x in v[-16:]])
if v: s=sorted(v); print("min %.1f median %.1f max %.1f ms" % (s[0], s[len(s)//2], s[-1]))'
systemctl show celikpanel-release-recovery.service -p CPUAccounting,CPUUsageNSec,ExecMainStartTimestamp,Result | cat
