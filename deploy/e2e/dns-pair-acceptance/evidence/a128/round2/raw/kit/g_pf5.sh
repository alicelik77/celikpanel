date -u +%T
ls -la --time-style=+%T /etc/postfix/celikpanel-postfwd/; cat /etc/postfix/celikpanel-postfwd/paused.list 2>&1 | grep -v "^#"
grep -n "PAUSE" /etc/postfix/postfwd.cf | cut -c1-200
journalctl -u postfwd --since 05:40:30 -o cat --no-pager | grep -vE "selftest" | grep -iE "reload|HUP|box1|paus" | tail -8 | cut -c1-260
journalctl -u celikpanel-panel -u celikpanel-agent --since 05:40:30 -o cat --no-pager | grep -iE "outgoing|paus|postfwd|resum" | tail -10 | cut -c1-300
