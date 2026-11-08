# guest (root, read-only): why systemd is still starting. Lab only.
systemctl is-system-running; systemctl list-jobs --no-pager | head -8
journalctl -u cloud-config.service -b --no-pager -o short-iso | tail -8 | cut -c1-250
ps -eo pid,etimes,args --sort=start_time | grep -iE "cloud|apt|dpkg|modules" | grep -v grep | cut -c1-200 | head
tail -5 /var/log/cloud-init-output.log 2>/dev/null | cut -c1-250
