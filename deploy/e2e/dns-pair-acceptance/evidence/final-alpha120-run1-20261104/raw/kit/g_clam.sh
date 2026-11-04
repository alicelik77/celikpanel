# guest (root, read only): ClamAV install evidence. Lab only.
echo "## $(date -u +%T)"; systemctl is-active clamav-daemon clamav-daemon.socket clamav-freshclam
systemctl status clamav-daemon --no-pager 2>&1 | sed -n 1,10p | cut -c1-250
ls -la --time-style=+%T /var/lib/clamav/
journalctl -u clamav-freshclam -u clamav-daemon -u clamav-daemon.socket --no-pager -o short-precise 2>/dev/null | head -30 | cut -c1-250
journalctl -u celikpanel-agent --no-pager -o short-precise --since "16:43:50" --until "16:44:30" 2>/dev/null | grep -iE "clam|freshclam|signature|install" | head -20 | cut -c1-300
cat /etc/systemd/system/clamav-daemon.service.d/*.conf 2>/dev/null | head -20
