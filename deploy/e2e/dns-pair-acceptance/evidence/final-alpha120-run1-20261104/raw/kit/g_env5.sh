# guest (root, read only): service-app env file location and modes, process environment checks. Lab only.
u=celiksvc-1.service
echo "## $(date -u +%T) $u active=$(systemctl is-active $u) pid=$(systemctl show $u -p MainPID --value)"
echo "## EnvironmentFiles: $(systemctl show $u -p EnvironmentFiles --value)"; grep -n "EnvironmentFile" /etc/systemd/system/$u 2>&1
stat -c '%n %U:%G %a %F' /etc/celikpanel-svcapps /etc/celikpanel-svcapps/* 2>&1
h=$(systemctl show $u -p WorkingDirectory --value); echo "## working dir $h"; ls -la "$h" "$h/config" 2>&1 | head -12
pid=$(systemctl show $u -p MainPID --value)
if [ "$pid" != 0 ]; then
  echo "## /proc/$pid/environ: $(tr '\0' '\n' < /proc/$pid/environ | wc -l) vars; LAB_CANARY: $(tr '\0' '\n' < /proc/$pid/environ | grep -c LAB_CANARY); CELIKPANEL_ vars: $(tr '\0' '\n' < /proc/$pid/environ | grep -c '^CELIKPANEL_')"
  for c in $(pgrep -P $pid); do echo "   child $c LAB_CANARY: $(tr '\0' '\n' < /proc/$c/environ 2>/dev/null | grep -c LAB_CANARY)"; done
fi
journalctl -u celikpanel-agent --no-pager --since -10min 2>/dev/null | grep -iE "svcapp|service app|env_edited|link|symlink|celiksvc" | tail -6 | cut -c1-300
