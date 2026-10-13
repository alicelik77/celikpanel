#!/bin/bash
echo "date_utc=$(date -u +%FT%TZ) whoami=$(id -u)"
echo "qemu=$(pgrep -c qemu-system || true)"
pgrep -af 'cp-set1|run-set|lab.py|_trial.py' | head -n 20 || true
df -h /var/tmp | tail -n 1
free -m | sed -n 2p
nproc
ls -d /var/tmp/cp-* 2>/dev/null
ls /var/tmp/cp-pair-accept/dist/ 2>/dev/null | tail -n 12
ls /var/tmp/cp-v3n28/images 2>/dev/null
ls -d /opt/celikpanel-test-toolchains/go1.26.5/go/bin && /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version
which node npm python3 qemu-system-x86_64 ssh 2>&1
