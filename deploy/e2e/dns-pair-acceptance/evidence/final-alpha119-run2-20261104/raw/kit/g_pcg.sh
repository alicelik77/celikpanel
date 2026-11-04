# guest (root, read only): evidence for "Failed to parse ProtectControlGroups: strict". Lab only.
systemctl --version | head -1; grep PRETTY /etc/os-release
journalctl -u celikpanel-agent --no-pager --since -10min 2>/dev/null | grep -iE "ProtectControlGroups|build|venv|systemd-run" | tail -8 | cut -c1-300
echo "## systemd-run property parse check (no unit started: --dry-run is not available, so this only parses via show)"
systemd-run --user --help >/dev/null 2>&1; man -w systemd.exec >/dev/null 2>&1
grep -rl "ProtectControlGroups=strict" /etc/systemd/system/ 2>/dev/null | head -3
