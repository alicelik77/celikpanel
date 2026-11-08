# lab: the Debian 13 lab image's cloud-init hangs after a reboot (a129 D5); end it so boot finishes (lab only)
systemctl stop cloud-config.service cloud-final.service 2>&1 | tail -1; pkill -f "cloud-init --all-stages"; sleep 8
echo "is-system-running: $(systemctl is-system-running)"; systemctl list-jobs --no-pager | head -4
touch /etc/cloud/cloud-init.disabled; echo "cloud-init disabled for later boots (lab)"
