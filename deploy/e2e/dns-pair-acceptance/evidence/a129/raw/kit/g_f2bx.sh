L1="2026-11-07 19:00:37 44 [Warning] Access denied for user 'u1'@'_gateway' (using password: YES)"
L2="2026-11-07 19:00:37 44 [Warning] Access denied for user 'u1'@'192.0.3.1' (using password: YES)"
L3="host1 mariadbd[123]: 2026-11-07 19:00:37 44 [Warning] Access denied for user 'u1'@'192.0.3.1' (using password: YES)"
B="(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<HOST>' \(using password: (?:YES|NO)\)"
for v in "^$B *\Z" "^$B *\n?\Z" "^$B\s*\Z" "^$B *$" "$B *\n?\Z" "^\s*$B *\n?\Z"; do for L in "$L2" "$L3" "$L1"; do m=$(fail2ban-regex --usedns=no "$L" "$v" 2>&1 | grep -E "^Lines" | sed 's/.*ignored, //'); echo "${v: -12} | ${v:0:6} | ${L:0:6}..${L##*@} | $m"; done; done
echo "--- usedns=warn with _gateway:"; fail2ban-regex --usedns=warn "$L1" "^$B *\n?\Z" 2>&1 | grep -E "^Lines|Unable|WARN" | head -3
