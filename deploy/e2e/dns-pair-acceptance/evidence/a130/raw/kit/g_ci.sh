journalctl -u cloud-config.service -b -o short-iso --no-pager | tail -5 | cut -c1-250
ps -eo pid,etime,args --forest | grep -A3 -B1 "cloud-init" | grep -v grep | head -12 | cut -c1-200
if [ "$1" = stop ]; then systemctl stop cloud-config.service cloud-final.service; sleep 5; echo "after stop: $(systemctl is-system-running)"; fi
