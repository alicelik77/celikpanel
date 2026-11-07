export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
df -h / /var/tmp | cat; free -m | head -2; nproc
echo "--- qemu"; pgrep -af qemu-system | grep -o "name [^ ]*" ; echo "(end)"
echo "--- /var/tmp"; du -sh /var/tmp/* 2>/dev/null | sort -h | tail -25
echo "--- cp-beta1"; ls /var/tmp/cp-beta1 /var/tmp/cp-beta1/state /var/tmp/cp-beta1/lab 2>&1 | head -40
ls /var/tmp/cp-beta1/lab/cells /var/tmp/cp-beta1/lab/images 2>&1 | head; ls -la /var/tmp/cp-beta1/lab/ 2>&1 | head -30
echo "--- dist"; ls /var/tmp/cp-pair-accept/dist 2>&1
ls /var/tmp/cp-v3n28 2>&1 | head
