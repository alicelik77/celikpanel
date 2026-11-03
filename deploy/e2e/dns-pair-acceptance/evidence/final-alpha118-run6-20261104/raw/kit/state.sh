#!/bin/bash
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
date -u
ls /var/tmp/ | head -40
echo "-- cp-beta1"; ls /var/tmp/cp-beta1 /var/tmp/cp-beta1/state /var/tmp/cp-beta1/lab/cells 2>&1
echo "-- dist"; ls /var/tmp/cp-pair-accept/dist 2>&1
echo "-- qemu"; pgrep -af qemu | cut -c1-200
echo "-- images"; ls -la /var/tmp/cp-v3n28/images 2>&1 | head
df -h /var/tmp | tail -1; free -g | head -2; nproc
ls /opt/celikpanel-test-toolchains/ 2>&1
which node npm go python3
