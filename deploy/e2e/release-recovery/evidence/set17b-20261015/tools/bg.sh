#!/bin/bash
# set17b. usage: bg.sh NAME script-file LIMIT_SECONDS -> runs the script detached under `timeout`, logs under $L/NAME.*
set -u
source <redacted>
name=$1; script=$2; limit=$3
mkdir -p $L
[ -e $L/$name.start ] && { echo "refusing: $name already started"; exit 2; }
tr -d '\r' < "$script" > $R/job-$name.sh
date -u +%FT%TZ > $L/$name.start
echo "$(date -u +%FT%TZ) start $name (limit ${limit}s)" >> $R/progress.txt
setsid nohup bash -c "cd $R; timeout --kill-after=60 $limit bash $R/job-$name.sh > $L/$name.out 2> $L/$name.err; echo \$? > $L/$name.rc; date -u +%FT%TZ > $L/$name.end; echo \"\$(date -u +%FT%TZ) end $name rc=\$(cat $L/$name.rc)\" >> $R/progress.txt" < /dev/null > /dev/null 2>&1 &
disown
sleep 2
echo "started $name"
