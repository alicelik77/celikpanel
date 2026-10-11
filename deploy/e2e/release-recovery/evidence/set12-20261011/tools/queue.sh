#!/bin/bash
# set12: run cells one at a time; after each, stage its evidence (stage-lab.sh removes that lab's overlays).
# usage: queue.sh "job:lab:node:group" ...   (run through bg.sh; SET12_COPY from the caller)
T=<scratchpad>/set12/tools
R=/var/tmp/cp-set12-run; L=$R/logs
for spec in "$@"; do
  IFS=: read -r job lab node group <<< "$spec"
  [ -e $L/$job.start ] && { echo "skip $job: already started"; continue; }
  tr -d '\r' < $T/job-$job.sh > $R/job-$job.sh
  date -u +%FT%TZ > $L/$job.start; echo "$(date -u +%FT%TZ) start $job (queue)" >> $R/progress.txt
  bash $R/job-$job.sh > $L/$job.out 2> $L/$job.err; echo $? > $L/$job.rc; date -u +%FT%TZ > $L/$job.end
  echo "$(date -u +%FT%TZ) end $job rc=$(cat $L/$job.rc)" >> $R/progress.txt
  bash $T/stage-lab.sh $job $lab $node $group > $L/stage-$job.txt 2>&1; echo "$(date -u +%FT%TZ) staged $job rc=$?" >> $R/progress.txt
  [ -e $R/queue.stop ] && { echo "queue stopped after $job" >> $R/progress.txt; exit 0; }
done
