#!/bin/bash
# set13: show one lab run's cells (all expectations of the named cells). usage: show.sh LAB CELL...
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1); echo "$d"; shift
python3 -I <scratchpad>/set13/tools/cellshow.py "$d" "$@"
