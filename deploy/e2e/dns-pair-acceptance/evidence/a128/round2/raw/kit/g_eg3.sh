# guest (root, read-only): while a Node.js install step runs, its cgroup and the egress set, every second for 25 s. Lab only.
for i in $(seq 1 25); do
  st=$(systemctl show -P SubState celikappinst-3.service); cg=$(systemctl show -P ControlGroup celikappinst-3.service)
  inset=$(nft list set inet celikpanel_egress smtp_blocked 2>/dev/null | grep -c 'celikappinst-3')
  echo "$(date +%T) sub=$st cgroup=$cg in_set=$inset guard=$(systemctl show -P Result celikpanel-egress-guard.service)/$(systemctl show -P ActiveState celikpanel-egress-guard.service)"
  sleep 1
done | uniq -f1
