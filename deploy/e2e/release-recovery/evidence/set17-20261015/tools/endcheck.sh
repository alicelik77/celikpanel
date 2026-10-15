#!/bin/bash
# set17: the end of the run: time, what is left and how large (reads only).
echo "date_utc_end=$(date -u +%FT%TZ)"
echo "qemu=$(pgrep -c qemu-system || true)"
pgrep -af 'run-set17|set17_trial|lab.py|cold-load' | grep -v pgrep || echo "no set17 process runs"
du -sh /var/tmp/cp-set17 /var/tmp/cp-release-drill-s17-* 2>/dev/null
ls -d /var/tmp/cp-* 2>/dev/null
ls /var/tmp/cp-pair-accept/dist/
df -h / | cat
