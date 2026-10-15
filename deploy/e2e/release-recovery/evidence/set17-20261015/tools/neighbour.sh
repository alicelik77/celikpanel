#!/bin/bash
# set17: before a guest start, no other qemu-system process may run on this host. One check, no waiting.
source <redacted>
label=${1:-guest start}
q=$(pgrep -fc qemu-system || true)
echo "$(date -u +%FT%TZ) neighbour gate for $label qemu_processes=$q" >> $R/neighbour-gate.txt
[ "$q" = 0 ]
