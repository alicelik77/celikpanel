#!/bin/bash
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
df -h /var/tmp / | cat
ls -la /var/tmp/ | head -60
echo ==images; ls -la /var/tmp/cp-v3n28/images 2>/dev/null
echo ==dists; ls /var/tmp/cp-pair-accept/dist 2>/dev/null; du -sh /var/tmp/cp-pair-accept 2>/dev/null
echo ==a128r2; ls /var/tmp/a129o 2>/dev/null | head -80; du -sh /var/tmp/a129o 2>/dev/null
echo ==qemu; pgrep -a qemu | cut -c1-200
echo ==lab cells; ls /var/tmp/cp-v3n28 2>/dev/null
