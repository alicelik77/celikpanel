#!/bin/bash
# set11: stage the five fresh-install labs that have ended (Debian 13 runs a, b, c; Ubuntu; Arch).
R=/var/tmp/cp-set11-run
J=<scratchpad>/set11/tools
[ -f $R/evidence-name.txt ] || echo set11-20261011 > $R/evidence-name.txt
for spec in cell-d13:s11-d13-a:debian13:debian13/run-a cell-d13b:s11-d13-b:debian13:debian13/run-b cell-d13c:s11-d13-c:debian13:debian13/run-c cell-ub:s11-ub-a:ubuntu:ubuntu/run-a cell-arch:s11-arch-a:arch:arch/run-a; do
  IFS=: read job lab node group <<< "$spec"
  echo "=== $job $lab"
  bash $J/stage-lab.sh $job $lab $node $group 2>&1 | tail -n 6
done
