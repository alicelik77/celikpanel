#!/bin/bash
# set17b: wait INSIDE the WSL host, at most LIMIT seconds (default 540, never above 570), until job NAME has ended.
source <redacted>
n=$1; limit=${2:-540}; t=0
[ "$limit" -le 570 ] || limit=570
until [ -e $L/$n.end ] || [ $t -ge $limit ]; do sleep 10; t=$((t+10)); done
echo "job=$n waited=${t}s end=$(cat $L/$n.end 2>/dev/null) rc=$(cat $L/$n.rc 2>/dev/null)"
tail -n 6 $L/$n.out 2>/dev/null | cut -c1-300
