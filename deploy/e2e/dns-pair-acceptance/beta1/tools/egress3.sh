echo "== unbound journal: denied-name questions (client, name, type)"; journalctl -u unbound --no-pager | grep -iE 'celikpanel|letsencrypt|lencr' | grep -oE 'info: [0-9.]+ [^ ]+ [A-Z]+ IN' | sort | uniq -c | sort -rn | head
journalctl -u unbound --no-pager | grep -iE 'celikpanel' | grep -iE 'reply|NXDOMAIN' | head -3
