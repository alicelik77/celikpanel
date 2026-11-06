# guest (root, read-only): CPU of each recovery watchdog run, read from systemd after it ends (CPUUsageNSec of the
# last invocation), for $1 seconds; plus whether the run took the idle fast path (stamp unchanged). Lab only.
U=celikpanel-release-recovery.service; last=""; n=$(( ${1:-180} / 4 ))
echo "stamp now: $(cat /run/celikpanel-release-recovery/idle-verified 2>&1)"
for i in $(seq 1 $n); do
  s=$(systemctl show $U -p ExecMainStartTimestamp --value); st=$(systemctl show $U -p ActiveState --value)
  if [ "$s" != "$last" ] && [ "$st" = inactive ]; then c=$(systemctl show $U -p CPUUsageNSec --value); echo "run $s cpu_ms $(( c / 1000000 )) result $(systemctl show $U -p Result --value) stamp_uptime $(cut -d' ' -f3 /run/celikpanel-release-recovery/idle-verified 2>/dev/null)"; last="$s"; fi
  sleep 4
done
