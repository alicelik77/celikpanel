#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-s15-u83-good-c/evidence/debian13/upd1/*/ | tail -1)
ls -la --time-style=+%T $d/steps | tail -5
last=$(ls -d $d/steps/* | tail -1); echo "last=$last"; ls -la --time-style=+%T $last | tail -8
tail -n 5 /var/tmp/cp-set15-run/logs/ugoodc.err | cut -c1-300
pgrep -af 'set15_trial|ssh .*516[7]' | cut -c1-200 | head
date -u +%T
