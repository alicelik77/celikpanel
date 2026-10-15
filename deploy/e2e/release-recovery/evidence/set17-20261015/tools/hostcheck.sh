#!/bin/bash
echo "date_utc=$(date -u +%FT%TZ) uid=$(id -u)"
echo "qemu=$(pgrep -c qemu-system || true)"
pgrep -af 'cp-set1|run-set|lab.py|_trial.py|cold-load' | grep -v pgrep | head -n 20 || true
df -h / /var/tmp | cat
free -m | sed -n 2p
nproc
ls -l /dev/kvm; echo "svm/vmx processors: $(grep -c -E ' (vmx|svm) ' /proc/cpuinfo) of $(nproc)"
ls -d /var/tmp/cp-* 2>/dev/null
ls /var/tmp/cp-pair-accept/dist/ 2>/dev/null | wc -l
ls /var/tmp/cp-v3n28/images 2>/dev/null
/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version
which node npm python3 qemu-system-x86_64 ssh 2>&1
