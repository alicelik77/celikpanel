journalctl --no-pager --since -3min 2>/dev/null | grep -iE 'joomla|installation/joomla.php|PHP (Fatal|Warning)' | tail -12 | cut -c1-400
ls /var/www/celikpanel/subscriptions/4/sites/15/ 2>&1 | head; ls /var/www/celikpanel/subscriptions/4/sites/15/public_html/installation 2>&1 | head -5
