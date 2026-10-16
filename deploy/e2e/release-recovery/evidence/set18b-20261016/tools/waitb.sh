#!/bin/bash
# set18b: wait INSIDE WSL, at most LIMIT seconds (<= 570), until the browser's events.jsonl has a line matching PATTERN
# (or the browser log has an rc= line); prints the last events and the newest states.  usage: waitb.sh PATTERN [LIMIT]
source <redacted>
B=$J/browser/h18ba-setup; pat=$1; limit=${2:-540}; t=0
[ "$limit" -le 570 ] || limit=570
until grep -qE "$pat" $B/events.jsonl 2>/dev/null || grep -q '^rc=' $J/browser/h18ba-setup.log || [ $t -ge $limit ]; do sleep 5; t=$((t+5)); done
echo "waited=${t}s now=$(date -u +%T) match=$(grep -cE "$pat" $B/events.jsonl 2>/dev/null)"
grep '^rc=' $J/browser/h18ba-setup.log
tail -n 6 $B/events.jsonl | cut -c1-260
for f in $B/c1-samples.jsonl $B/c2-samples.jsonl; do [ -f $f ] && { echo "== $(basename $f)"; python3 -I $J/states.py $f | tail -n ${3:-6} | cut -c1-330; }; done
