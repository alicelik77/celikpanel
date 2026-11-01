echo "-- all postfix lines 16:58:40-16:59:20"
journalctl -u postfix -u 'postfix@*' --no-pager -o short-iso --since '2026-11-01 16:58:40' --until '2026-11-01 16:59:20' | cut -c1-300
echo "-- postfix 'changed' / reload lines in the window 16:57-17:02"
journalctl -u postfix -u 'postfix@*' --no-pager -o short-iso --since '2026-11-01 16:57:00' --until '2026-11-01 17:02:30' | grep -iE 'changed|reload|restarting|refresh' | cut -c1-250
echo "-- agent lines 16:58:40-16:59:10"
journalctl -u celikpanel-agent --no-pager -o short-iso --since '2026-11-01 16:58:40' --until '2026-11-01 16:59:10' | cut -c1-330 | tail -30
echo "-- panel lines 16:58:40-16:59:10"
journalctl -u celikpanel-panel --no-pager -o short-iso --since '2026-11-01 16:58:40' --until '2026-11-01 16:59:10' | cut -c1-330 | tail -10
ls -la --time-style=full-iso /etc/postfix/celikpanel_transport*
