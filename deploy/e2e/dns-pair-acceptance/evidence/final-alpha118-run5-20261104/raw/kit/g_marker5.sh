# guest (root, read only): quota-warning markers. Lab only.
ls -la /var/lib/celikpanel-quota-warning/ 2>&1; stat -c '%n %U:%G %a %F' /var/lib/celikpanel-quota-warning/* 2>&1
journalctl -u celikpanel-quota-warning.service --no-pager 2>/dev/null | tail -6 | cut -c1-250
