#!/bin/bash
# set11: wait until job NAME's output names step PATTERN (grep -E), or a browser window opens, or the job ends,
# at most LIMIT s; then print the step lines. usage: waituntil.sh NAME PATTERN [LIMIT]
L=/var/tmp/cp-set11-run/logs; n=$1; pat=${2//,/|}; limit=${3:-290}; t=0
w0=$(grep -c '"set11": "ready"' $L/$n.out 2>/dev/null)
until grep -qE "^\{\"step\": \"($pat)\"" $L/$n.out 2>/dev/null || [ -e $L/$n.end ] || [ "$(grep -c '"set11": "ready"' $L/$n.out 2>/dev/null)" != "$w0" ] || [ $t -ge $limit ]; do sleep 5; t=$((t+5)); done
echo "waited=${t}s"
grep -h '^{"step"\|"set11": "ready"' $L/$n.out 2>/dev/null | cut -c1-260 | tail -n 6
[ -e $L/$n.end ] && echo "ENDED rc=$(cat $L/$n.rc)"
date -u +%T
