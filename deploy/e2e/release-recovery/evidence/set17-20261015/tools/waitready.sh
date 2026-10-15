#!/bin/bash
# set17: wait INSIDE the WSL host, at most LIMIT seconds (default 540), until the driver of job NAME has opened its
# browser window (hand/<prefix>-$W/ready.json) or the job has ended; prints what it found. usage: waitready.sh NAME PREFIX [LIMIT] [WINDOW]
source <redacted>
n=$1; prefix=$2; limit=${3:-540}; W=${4:-ui}; t=0
[ "$limit" -le 570 ] || limit=570
until [ -e $R/hand/$prefix-$W/ready.json ] || [ -e $L/$n.end ] || [ $t -ge $limit ]; do sleep 10; t=$((t+10)); done
echo "job=$n waited=${t}s ready=$([ -e $R/hand/$prefix-$W/ready.json ] && echo yes || echo no) end=$(cat $L/$n.end 2>/dev/null) rc=$(cat $L/$n.rc 2>/dev/null)"
grep -h '^{"step"' $L/$n.out 2>/dev/null | cut -c1-200 | tail -n 6
tail -n 3 $L/$n.err 2>/dev/null | cut -c1-300
