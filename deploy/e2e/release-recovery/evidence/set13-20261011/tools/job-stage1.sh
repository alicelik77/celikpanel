#!/bin/bash
# set13: stage the three finished labs (Debian run a, Ubuntu runs a and b).
J=<scratchpad>/set13/tools
R=/var/tmp/cp-set13-run
[ -f $R/evidence-name.txt ] || echo set13-20261011 > $R/evidence-name.txt
bash $J/stage-lab.sh d13 s13-d13-a debian13 debian13/fresh-a
bash $J/stage-lab.sh ub s13-ub-a ubuntu ubuntu/fresh-a
bash $J/stage-lab.sh ubb s13-ub-b ubuntu ubuntu/fresh-b
