#!/bin/bash
# set11: read-only look at the WSL QEMU host before anything is created.
echo "date_utc=$(date -u +%FT%TZ) user=$(id -un) kernel=$(uname -r)"
echo "qemu-system procs: $(pgrep -fc qemu-system)"; pgrep -af qemu-system | cut -c1-160
echo "--- set10/set11 run dirs"; ls -la /var/tmp/cp-set10-run 2>&1 | head -5; ls -la /var/tmp/cp-set11-run 2>&1 | head
echo "--- live jobs of earlier runs"; pgrep -af '/var/tmp/cp-set(9|10)-run' | cut -c1-160 || echo "(none)"
echo "--- image cache"; ls -la /var/tmp/cp-v3n28/images 2>&1
echo "--- drills"; ls -d /var/tmp/cp-release-drill-* 2>/dev/null | tail -n 20
echo "--- dist"; ls -la /var/tmp/cp-pair-accept/dist 2>&1 | tail -n 12
echo "--- builds"; ls -la /var/tmp/cp-upd1-build 2>&1 | tail -n 8
echo "--- go"; /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version
echo "--- df"; df -h /var/tmp | tail -1
free -m | sed -n 2p
qemu-system-x86_64 --version | head -n1
python3 --version
