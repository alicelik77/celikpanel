mkdir -p /tmp/f2bt
cat > /tmp/f2bt/new.conf <<'F'
[Definition]
failregex = ^ *(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<HOST>' \(using password: (?:YES|NO)\) *\n?\Z
ignoreregex =
F
echo "--- the real journal lines (11 refused sign-ins, host _gateway as MariaDB resolved it):"; fail2ban-regex --usedns=no /tmp/f2bt/denied.log /tmp/f2bt/new.conf 2>&1 | grep -E "^Lines"
sed "s/'_gateway'/'192.0.3.1'/" /tmp/f2bt/denied.log > /tmp/f2bt/denied-ip.log
echo "--- the same lines with the address MariaDB logs when it does not resolve names:"; fail2ban-regex --usedns=no /tmp/f2bt/denied-ip.log /tmp/f2bt/new.conf 2>&1 | grep -E "^Lines|^\|  *[0-9]|192.0.3.1" | head -4
echo "--- the shipped (old) regex on the same address lines:"; printf '[Definition]\nfailregex = %s\nignoreregex =\n' '^(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '"'"'[^'"'"'\n]*'"'"'@'"'"'<HOST>'"'"' \(using password: (?:YES|NO)\) *\Z' > /tmp/f2bt/old.conf; fail2ban-regex --usedns=no /tmp/f2bt/denied-ip.log /tmp/f2bt/old.conf 2>&1 | grep -E "^Lines"
fail2ban-client --version 2>&1 | head -1
