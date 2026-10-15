#!/bin/bash
# set17b: the end of the run: time, what is left and how large (reads only).
echo "date_utc_end=$(date -u +%FT%TZ)"
echo "qemu=$(pgrep -c qemu-system || true)"
pgrep -af 'run-set17|set17_trial|set17b_trial|lab.py|cold-load|restart_delay' | grep -v pgrep || echo "no set17b process runs"
du -sh /var/tmp/cp-set17b /var/tmp/cp-release-drill-s17b-* 2>/dev/null
ls -d /var/tmp/cp-* 2>/dev/null
ls /var/tmp/cp-pair-accept/dist/
df -h / | cat
