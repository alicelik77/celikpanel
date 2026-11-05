# guest (root, read-only): the systemd private socket the installer checks. Lab only.
grep PRETTY /etc/os-release; uname -r; systemctl --version | head -1
stat -c '%A %a %U:%G links=%h %n' /run/systemd /run/systemd/private
ls -la /root/celikpanel-install.sh 2>/dev/null; ls -d /opt/celikpanel /etc/celikpanel 2>&1
systemctl is-system-running
