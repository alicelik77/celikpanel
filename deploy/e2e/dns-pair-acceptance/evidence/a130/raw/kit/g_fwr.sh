# guest (root): lab — the owner restores the panel firewall table lost to the case-4.10 `nft flush ruleset`
# with the panel's own restore unit, then reads the state. Lab only.
U=$(systemctl list-unit-files --no-legend | awk '{print $1}' | grep -E '^celikpanel.*firewall.*\.service$' | grep -v database-remote | head -1); echo "unit $U"
systemctl restart "$U"; sleep 2; systemctl is-active "$U"; nft list tables | xargs
nft list table inet celikpanel_dbremote | grep -E "saddr" | sed 's/^\s*/   /'
