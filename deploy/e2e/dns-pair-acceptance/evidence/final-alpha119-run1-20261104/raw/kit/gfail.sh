# guest (root, read only): Joomla setup failure and ClamAV daemon state. Lab only.
echo "## $(date -u +%T) agent journal: joomla"; journalctl -u celikpanel-agent --no-pager --since -15min 2>/dev/null | grep -iE 'joomla|catalog|cli install|exit code 255' | tail -15 | cut -c1-400
echo "## clamav"; systemctl is-active clamav-daemon clamav-freshclam; systemctl status clamav-daemon --no-pager 2>&1 | sed -n 1,12p | cut -c1-250
ls -la /var/lib/clamav/ 2>&1 | head; journalctl -u clamav-freshclam --no-pager 2>/dev/null | tail -8 | cut -c1-250
free -m | head -2
