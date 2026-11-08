grep -A3 "Start-Date: 2026-11-08  10:4[5-6]" /var/log/apt/history.log | head -12
D=$(ls /var/cache/apt/archives/postfix_*.deb 2>/dev/null | head -1); echo "deb: $D"
if [ -z "$D" ]; then cd /tmp && apt-get download postfix >/dev/null 2>&1; D=$(ls /tmp/postfix_*.deb | head -1); fi
mkdir -p /tmp/pfx && dpkg-deb -e "$D" /tmp/pfx && grep -n "service.d\|systemd/system" /tmp/pfx/postrm | head; rm -rf /tmp/pfx /tmp/postfix_*.deb
journalctl --since "2026-11-08 10:45:15" --until "2026-11-08 10:46:05" -o cat --no-pager | grep -iE "service.d|owner.conf|drop-in" | head
