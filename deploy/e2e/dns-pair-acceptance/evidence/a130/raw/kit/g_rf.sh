systemctl reset-failed lab-hang.service 2>/dev/null; systemctl daemon-reload; echo "is-system-running: $(systemctl is-system-running)"; systemctl --failed --no-legend --plain | cat
