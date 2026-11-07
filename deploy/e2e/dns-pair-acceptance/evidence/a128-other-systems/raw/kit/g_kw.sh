systemctl --version | head -1
systemctl kill --kill-whom=main --signal=SIGCONT cron.service; echo "kill-whom rc $?"
systemctl kill --kill-who=main --signal=SIGCONT cron.service; echo "kill-who rc $?"
grep -rn "kill-whom" /etc/logrotate.d/ 2>/dev/null | cut -c1-200
