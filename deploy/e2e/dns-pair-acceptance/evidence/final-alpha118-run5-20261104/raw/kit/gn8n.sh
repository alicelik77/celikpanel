# guest (root, read only): service app units, listeners, hardening env (values of secrets not printed). Lab only.
echo "## $(date -u +%T) units"; systemctl list-units 'celiksvc*' --all --no-pager --no-legend 2>&1 | head
for u in $(systemctl list-units 'celiksvc-*' --all --no-legend --plain 2>/dev/null | awk '{print $1}' | grep -v inst); do
  echo "## $u"; systemctl is-active $u; systemctl is-enabled $u 2>&1
  systemctl show $u -p Environment 2>/dev/null | tr ' ' '\n' | grep -E 'NODES_EXCLUDE|N8N_LISTEN_ADDRESS|N8N_PORT|N8N_BLOCK_ENV|N8N_COMMUNITY|N8N_PYTHON|N8N_RESTRICT' | head -10
  f=$(systemctl show $u -p EnvironmentFiles --value 2>/dev/null | awk '{print $1}'); [ -n "$f" ] && grep -E 'NODES_EXCLUDE|N8N_LISTEN_ADDRESS|N8N_PORT=' "$f" 2>/dev/null
  pid=$(systemctl show $u -p MainPID --value); echo "pid $pid"
done
echo "## listeners 21000-29999"; ss -ltnp 2>/dev/null | awk '{print $4, $6}' | grep -E ':(2[1-9][0-9]{3}) ' | head
echo "## memory"; free -m | head -2; df -h / | tail -1
