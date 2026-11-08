echo "is-system-running: $(systemctl is-system-running)"; systemctl list-jobs --no-pager | head -8; systemctl status lab-hang.service --no-pager 2>&1 | head -4
