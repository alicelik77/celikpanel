#!/bin/bash
# set19: before a guest start, no other qemu-system process may run on this host. One check, no waiting.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
q=$(pgrep -fc qemu-system || true)
echo "$(date -u +%FT%TZ) neighbour gate for ${1:-guest start} qemu_processes=$q" >> $R/neighbour-gate.txt
[ "$q" = 0 ]
