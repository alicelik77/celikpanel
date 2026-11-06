# guest (root, read-only): a Node.js site's unit, its listener and its sandbox marker. $1 = domain id. Lab only.
U=celikapp-$1.service
echo "unit $U active=$(systemctl is-active $U) sha=$(sha256sum /etc/systemd/system/$U | cut -c1-16)"
head -3 /etc/systemd/system/$U; grep -nE "celikpanel-sandbox-generation|^User=|^Environment=(PORT|HOST)|SocketBind|IPAddress|PrivateTmp|ProtectSystem|ProtectHome|NoNewPrivileges|RestrictAddressFamilies|ReadWritePaths|InaccessiblePaths|TemporaryFileSystem|BindReadOnlyPaths|Slice" /etc/systemd/system/$U | cut -c1-200
ls /etc/systemd/system/$U.d 2>/dev/null
P=$(systemctl show -p MainPID --value $U); echo "main pid $P"; ss -ltnp | grep -E "pid=($P|$(pgrep -P $P | tr '\n' '|')0)," | sed 's/  */ /g'
nft list set inet celikpanel_egress smtp_blocked 2>/dev/null | grep -o "celikapp-$1" | head -1
