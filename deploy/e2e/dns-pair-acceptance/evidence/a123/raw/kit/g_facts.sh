# guest (root, read-only): host facts for the sandbox level. Lab only.
grep PRETTY /etc/os-release; uname -r; systemctl --version | head -1; nft --version 2>&1; 
echo "cgroup: $(stat -fc %T /sys/fs/cgroup)"; echo "bpf lsm: $(cat /sys/kernel/security/lsm 2>/dev/null)"
echo "panel: $(cat /opt/celikpanel/release.commit 2>/dev/null || cat /opt/celikpanel/*/release.commit 2>/dev/null | head -1)"
systemctl is-active celikpanel-agent.service celikpanel-panel.service | tr '\n' ' '; echo
