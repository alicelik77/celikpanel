# guest (root): Fail2ban status / unban the lab client. Lab only.
J=$(fail2ban-client status 2>/dev/null | sed -n 's/.*Jail list:\s*//p' | tr ',' ' ')
echo "jails: $J"
for j in $J; do
  if [ "$1" = unban ]; then fail2ban-client set $j unbanip 192.0.3.1 >/dev/null 2>&1; echo "unbanned in $j"
  else fail2ban-client status $j | grep -E "Currently failed|Total failed|Currently banned|Banned IP" | sed "s/^/$j: /"; fi
done
