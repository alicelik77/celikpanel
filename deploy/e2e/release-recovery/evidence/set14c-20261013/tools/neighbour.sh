#!/bin/bash
# set14c: before a guest start, no other measurement may run on this host: no qemu-system process of another lab and
# no job of an earlier run directory. One check, no waiting: a busy host refuses the start.
source <redacted>
label=${1:-guest start}
q=$(pgrep -fc qemu-system || true)
j=$(pgrep -fc '/var/tmp/cp-set1[0-4]b?-run/' || true)
echo "$(date -u +%FT%TZ) neighbour gate for $label qemu_processes=$q earlier_run_jobs=$j" >> $R/neighbour-gate.txt
[ "$q" = 0 ] && [ "$j" = 0 ]
