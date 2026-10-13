#!/bin/bash
# set14b: the first offline-suite output of run copy b (before the harness correction) and the first overlay record.
source <redacted>
E="$REPO/deploy/e2e/release-recovery/evidence/set14b-20261013"
cp $L/suite-b-first.txt "$E/harness-run-copy/b/suite-first.txt"; cp $L/mkov-b-first.txt "$E/harness-run-copy/b/mkov-first.txt"
cp $R/progress.txt "$E/host/progress.txt"
ls /var/tmp/cp-pair-accept/dist/ | grep -c . ; ls -la --time-style=+%FT%T /var/tmp/cp-pair-accept/dist/ | awk '$6 >= "2026-10-13T05:5"' 
