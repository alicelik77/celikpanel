grep -v "^#" /etc/postfix/postfwd.cf
for f in /etc/postfix/celikpanel-postfwd/*; do echo "## $f"; grep -v "^#" $f | head -5; done
journalctl -u postfwd --since 05:10:50 -o cat --no-pager | grep -v selftest | tail -8 | cut -c1-300
