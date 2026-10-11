#!/bin/bash
# set12: wait until job NAME has printed more than N step lines, or ended, or LIMIT seconds passed; then show them.
L=/var/tmp/cp-set12-run/logs; n=$1; want=${2:-1}; limit=${3:-1500}; t=0
count() { c=$(grep -c '^{"step"' $L/$n.out 2>/dev/null); echo ${c:-0}; }
until [ -e $L/$n.end ] || [ "$(count)" -gt "$want" ] || [ $t -ge $limit ]; do sleep 15; t=$((t+15)); done
echo "job=$n steps=$(count) end=$(cat $L/$n.end 2>/dev/null) rc=$(cat $L/$n.rc 2>/dev/null) waited=${t}s"
grep -h '^{"step"' $L/$n.out 2>/dev/null | cut -c1-260 | tail -n 8
grep -h '"set12": "ready"' $L/$n.out 2>/dev/null | tail -n 2
tail -n 2 $L/$n.err 2>/dev/null | cut -c1-300
