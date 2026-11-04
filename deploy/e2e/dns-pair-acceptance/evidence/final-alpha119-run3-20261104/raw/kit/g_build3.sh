# guest (root, read only): the Python build units. Lab only.
for u in celikrtbuild-1 celikrtbuild-2 celikrtbuild-3; do echo "## $u: $(systemctl is-active $u.service 2>&1) / $(systemctl show $u.service -p Result,ExecMainStatus,LoadState --value 2>&1 | tr '\n' ' ')"; journalctl -u $u.service --no-pager -o short-precise 2>/dev/null | tail -25 | cut -c1-320; done
systemctl list-units --all --no-legend --plain 'celikrt*' 2>&1 | head
ls -la /var/www/celikpanel/subscriptions/4/sites/4/app/ 2>&1
journalctl -u celikpanel-agent --no-pager --since -5min 2>/dev/null | grep -iE "build|venv|python" | tail -8 | cut -c1-300
