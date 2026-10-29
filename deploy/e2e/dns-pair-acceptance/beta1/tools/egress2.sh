echo "== non-NTP forward drops"; journalctl -k --no-pager | grep -E 'beta1-fwd-drop' | grep -v 'DPT=123 ' | grep -oE 'SRC=[0-9.]+ DST=[0-9.]+ .*PROTO=[A-Z]+( SPT=[0-9]+ DPT=[0-9]+)?' | awk '{print $1, $2, $(NF)}' | sort | uniq -c | sort -rn | head
ls -la /var/log/unbound/ /etc/unbound/var 2>&1 | head; find / -name beta1.log -path '*unbound*' 2>/dev/null | head -2
f=$(find / -name beta1.log -path '*unbound*' 2>/dev/null | head -1); [ -n "$f" ] && grep -ciE 'celikpanel' $f && grep -iE 'celikpanel|letsencrypt' $f | awk '{print $(NF-2), $(NF-1)}' | sort | uniq -c | head
journalctl -u unbound --no-pager | grep -ciE 'celikpanel'
