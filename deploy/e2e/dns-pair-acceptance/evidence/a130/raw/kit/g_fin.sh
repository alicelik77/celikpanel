echo "--- systemctl --failed"; systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"
echo "--- PHP switch refused while nginx was stopped (22:54:39-22:54:42)"
journalctl -u celikpanel-panel -u celikpanel-agent --since 22:54:39 --until 22:54:42 -o short-iso --no-pager | grep -iE "php|vhost|pool|500" | cut -c1-400
