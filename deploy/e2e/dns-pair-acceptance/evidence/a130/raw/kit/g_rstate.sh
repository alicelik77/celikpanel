# guest (root, read-only): remote access state for port $1. Lab only.
echo "--- nft tables: $(nft list tables 2>&1 | xargs)"
nft list table inet celikpanel_dbremote 2>&1 | grep -E "dport|saddr" | head -6 | sed 's/^\s*/    /'
echo "--- listeners:"; ss -ltn | grep -E ":$1 " | head -3
echo "--- units: $(systemctl is-active celikpanel-database-remote-firewall.service celikpanel-database-remote-firewall.timer 2>&1 | xargs) ; failed: $(systemctl list-units --failed --no-legend --plain | awk '{print $1}' | xargs)"
