# guest (root, read only): n8n unit state, listeners, native module errors in its journal. Lab only.
echo "## $(date -u +%T) units"; systemctl list-units 'celiksvc*' --all --no-pager --no-legend 2>&1 | head -4
u=celiksvc-1.service; echo "## $u active=$(systemctl is-active $u) enabled=$(systemctl is-enabled $u 2>&1) pid=$(systemctl show $u -p MainPID --value)"
echo "## listeners 21000-29999"; ss -ltnp 2>/dev/null | awk '{print $4, $6}' | grep -E ':(2[1-9][0-9]{3}) ' | head
echo "## env (hardening, no secrets)"; systemctl show $u -p Environment 2>/dev/null | tr ' ' '\n' | grep -E 'NODES_EXCLUDE|N8N_LISTEN_ADDRESS' | head -3
echo "## journal: native modules / errors"; journalctl -u $u --no-pager 2>/dev/null | grep -iE 'napi-rs|canvas|@napi|musl|glibc|ERR_DLOPEN|Cannot find module|Error:|error loading' | tail -10 | cut -c1-300
echo "## journal tail"; journalctl -u $u --no-pager 2>/dev/null | tail -6 | cut -c1-250
