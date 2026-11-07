pgrep -af qemu-system | grep a128ftp | tr ' ' '\n' | grep -E "netdev|mac|hostfwd|tap|ifname" | head
ls /var/tmp/a128-ftp-lab | head; ip -br addr | head -20
