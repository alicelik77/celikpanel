#!/bin/bash
echo "date_utc=$(date -u +%FT%TZ) uid=$(id -u)"
ls -l /dev/kvm; echo "svm/vmx procs: $(grep -c -E ' (vmx|svm) ' /proc/cpuinfo) of $(nproc)"
echo "qemu=$(pgrep -c qemu-system || true)"
df -h / /var/tmp | cat
free -m | sed -n 2p
ls -d /var/tmp/cp-* 2>/dev/null
for d in /var/tmp/cp-*; do du -sh "$d" 2>/dev/null; done
ls /var/tmp/cp-pair-accept/dist/
ls -la /var/tmp/cp-v3n28/images
