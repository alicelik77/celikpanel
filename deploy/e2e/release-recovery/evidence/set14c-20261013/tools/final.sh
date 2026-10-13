#!/bin/bash
echo "qemu=$(pgrep -c qemu-system || true) jobs=$(pgrep -fc 'cp-set14c-run|run-set14c|set14c_trial' || true)"
ls -d /var/tmp/* | sed -n '1,40p'
ls -la /var/tmp/cp-set14c-run/secret 2>&1 | tail -n +2
find /var/tmp/cp-release-drill-s14c-d13-a -name '*.qcow2' -o -name '*.img' | head
du -sh /var/tmp/cp-set14c-run /var/tmp/cp-release-drill-s14c-d13-a /var/tmp/cp-upd1-build/20261013t055405z 2>/dev/null
