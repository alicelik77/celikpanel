#!/bin/bash
# set13: the run directory, the host record before anything is created, and run copy a (pristine git archive 816b8dd60).
R=/var/tmp/cp-set13-run
J=<scratchpad>/set13/tools
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $R/logs $R/host $R/build
echo "$(date -u +%FT%TZ) set13 run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
G() { git -c safe.directory='*' -C '<repo>' "$@"; }
{ echo "HEAD $(G rev-parse HEAD)"; echo "816b8dd60 = $(G rev-parse '816b8dd60^{commit}')"; echo "git diff --stat 816b8dd60 HEAD -- cmd internal web/src:"; G diff --stat 816b8dd60 HEAD -- cmd internal web/src; echo "(end; empty = no product change after 816b8dd60)"; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh a > $R/logs/mkov-a.txt 2>&1; echo "mkov a rc=$?"; tail -n 4 $R/logs/mkov-a.txt
