#!/bin/bash
# set14b: run copy b was made before a harness correction (set14b_trial.py lent its names to set14_trial at import,
# which made set14's offline test fail in the same process); remove copy b and its overlay record, keep the first
# suite output as suite-b-first.txt, and run afterbuild.sh again.
source <redacted>
case $R in /var/tmp/cp-set14b-run) ;; *) exit 2;; esac
mv $L/suite-b.txt $L/suite-b-first.txt; mv $L/mkov-b.txt $L/mkov-b-first.txt
rm -rf -- $R/harness-b $R/overlay-b
echo "$(date -u +%FT%TZ) removed run copy b (made before a harness correction); making it again" >> $R/progress.txt
bash $J/afterbuild.sh
