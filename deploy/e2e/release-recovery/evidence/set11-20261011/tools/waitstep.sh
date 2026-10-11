#!/bin/bash
# set11: wait until job NAME prints a new step line, a browser window opens, or the job ends (at most LIMIT s), then poll.
L=/var/tmp/cp-set11-run/logs; n=$1; limit=${2:-290}; t=0
count() { grep -c '^{"step"\|"set11": "ready"' $L/$n.out 2>/dev/null || echo 0; }
start=$(count)
until [ "$(count)" != "$start" ] || [ -e $L/$n.end ] || [ $t -ge $limit ]; do sleep 5; t=$((t+5)); done
echo "waited=${t}s"
grep -h '^{"step"\|"set11": "ready"' $L/$n.out 2>/dev/null | cut -c1-240 | tail -n 4
[ -e $L/$n.end ] && echo "ENDED rc=$(cat $L/$n.rc)"
date -u +%T
