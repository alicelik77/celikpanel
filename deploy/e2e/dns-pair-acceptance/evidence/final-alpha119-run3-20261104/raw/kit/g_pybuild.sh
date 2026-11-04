# guest (root, read only): Python build failure evidence. Lab only.
A=/var/www/celikpanel/subscriptions/4/sites/4/app
echo "## $(date -u +%T) $A"; ls -la $A; ls -la $A/venv-1.partial 2>&1 | head; ls -la $A/venv-1.partial/bin 2>&1 | head
python3 --version; dpkg -l python3-venv python3.13-venv 2>/dev/null | grep -E "^ii" | awk '{print $2,$3}'
journalctl --no-pager --since "01:50:20" --until "01:50:40" 2>/dev/null | grep -iE "venv|celikrt|build|python|Failed to find" | head -20 | cut -c1-300
systemctl list-units --all --no-legend --plain 'celikrt*' 2>&1 | head
