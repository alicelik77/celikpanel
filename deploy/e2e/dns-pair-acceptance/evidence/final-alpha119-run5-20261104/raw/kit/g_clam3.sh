# guest (root, read only): ClamAV units, first-start path unit, signature files. Lab only.
echo "## $(date -u +%T)"; systemctl is-active clamav-daemon clamav-daemon.socket clamav-freshclam
for u in celikpanel-clamav-first-start.path celikpanel-clamav-first-start.service; do echo "## $u: active=$(systemctl is-active $u 2>&1) enabled=$(systemctl is-enabled $u 2>&1)"; ls -la /etc/systemd/system/$u 2>&1; done
journalctl -u celikpanel-clamav-first-start.path -u celikpanel-clamav-first-start.service --no-pager -o short-precise 2>/dev/null | tail -8 | cut -c1-250
ls -la --time-style=+%T /var/lib/clamav/
journalctl -u clamav-daemon -u clamav-daemon.socket --no-pager -o short-precise 2>/dev/null | grep -E "Started|skipped|Listening|Failed" | tail -8 | cut -c1-250
