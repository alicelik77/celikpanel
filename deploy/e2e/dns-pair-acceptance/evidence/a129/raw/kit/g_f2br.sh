# guest (root, read-only test): CelikPanel's MariaDB failregex against the real journal. Lab only.
mkdir -p /tmp/f2bt; cat > /tmp/f2bt/cp.conf <<'F'
[Definition]
failregex = ^(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<HOST>' \(using password: (?:YES|NO)\) *\Z
ignoreregex =
F
journalctl -u mariadb -o cat --since -2h --no-pager | grep "Access denied" > /tmp/f2bt/denied.log; wc -l < /tmp/f2bt/denied.log
for u in warn no yes; do echo "--- usedns=$u:"; fail2ban-regex --usedns=$u /tmp/f2bt/denied.log /tmp/f2bt/cp.conf 2>&1 | grep -E "^Lines|^Failregex|Unable|WARNING" | head -4; done
getent hosts _gateway; mariadb -N -e "select @@skip_name_resolve"
