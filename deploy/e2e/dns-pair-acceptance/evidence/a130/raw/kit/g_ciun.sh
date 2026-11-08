# guest (root): lab image only - cloud-init's config stage hangs after a reboot (Debian 13 image), which keeps
# systemd "starting". Record it, then stop the stuck boot job like an owner would. Lab only.
date -u +%T; systemctl is-system-running; systemctl list-jobs --no-pager --no-legend | cat
timeout 60 systemctl stop cloud-config.service; timeout 30 systemctl cancel 2>/dev/null; sleep 3
echo "after: $(systemctl is-system-running) ; jobs: $(systemctl list-jobs --no-legend | wc -l) ; failed: $(systemctl list-units --failed --no-legend --plain | awk '{print $1}' | xargs)"
