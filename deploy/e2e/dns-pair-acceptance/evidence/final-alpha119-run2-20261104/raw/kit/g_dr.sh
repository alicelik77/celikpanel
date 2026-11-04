R=/var/www/celikpanel/subscriptions/4/sites/14/public_html
echo "## $(date -u +%T) drupal docroot after failed removal"; ls -la $R 2>&1 | head -12; ls -la $R/sites/default 2>&1 | head -8
journalctl -u celikpanel-agent --no-pager --since -60min 2>/dev/null | grep -iE 'drupal|remove|rm -rf|Permission denied' | tail -8 | cut -c1-300
