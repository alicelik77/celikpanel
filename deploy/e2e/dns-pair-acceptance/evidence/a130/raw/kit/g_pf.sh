systemctl is-active postfix postfix@- ; systemctl show postfix@- -p ActiveEnterTimestamp -p InactiveEnterTimestamp -p NRestarts | cat
journalctl -u postfix@- -u postfix --since 11:35 -o short-iso --no-pager | grep -iE "stop|start|reload|fatal|terminat" | tail -12 | cut -c1-200
