# guest (root, read-only): release recovery journal of the forced failure + nginx site file times. Lab only.
journalctl --since "2026-11-08 09:59:15" --until "2026-11-08 10:01:05" -o short-iso --no-pager | grep -iE 'rollback|rolled back|restor|recovery_required|release recovery|Recovery dispatch|Verified snapshot|stopp' | cut -c1-260 | tail -25
echo "-- nginx site files"; ls -la --time-style=full-iso /etc/nginx/sites-available/
grep -n "autoconfig\|autodiscover\|celikpanel-mail" /etc/nginx/sites-available/cust1.s0.test.conf /etc/nginx/sites-available/site1.s0.test.conf /etc/nginx/sites-available/s0.test.conf | head -20
journalctl -u celikpanel-panel -u celikpanel-agent --since "2026-11-08 10:01:00" -o short-iso --no-pager | grep -iE "autoconfig|nginx|site config" | head -10 | cut -c1-260
