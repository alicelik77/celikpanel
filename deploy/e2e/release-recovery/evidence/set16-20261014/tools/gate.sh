#!/bin/bash
# set16: the disk rule before a guest start: the newest Windows C: reading (cwatch.ps1) younger than 180 s, at least 40 GiB.
source <redacted>
label=${1:-guest start}
reading=$(tail -n 1 $J/c-drive-watch.txt 2>/dev/null | tr -d '\r')
stamp=$(echo "$reading" | cut -d' ' -f1)
gib=$(echo "$reading" | sed -n 's/.*free_GiB=\([0-9.]*\).*/\1/p')
bytes=$(echo "$reading" | sed -n 's/.*free_bytes=\([0-9]*\).*/\1/p')
age=$(( $(date -u +%s) - $(date -u -d "$stamp" +%s 2>/dev/null || echo 0) ))
ok=yes
[ -n "$bytes" ] && [ "$bytes" -ge 42949672960 ] || ok=no
[ "$age" -le 180 ] || ok=no
[ ! -e $J/c-drive.below40 ] || ok=no
echo "$(date -u +%FT%TZ) before $label free_GiB=$gib free_bytes=$bytes (Get-PSDrive C reading of $stamp, ${age}s old) allowed=$ok" >> $R/c-drive-cells.txt
[ $ok = yes ]
