#!/bin/bash
# set12: the run directory, the host record before anything is created, and run copy a (pristine git archive 8fabc03b3).
R=/var/tmp/cp-set12-run
J=<scratchpad>/set12/tools
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $R/logs $R/host $R/build
echo "$(date -u +%FT%TZ) set12 run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
G() { git -c safe.directory='*' -C '<repo>' "$@"; }
{ echo "HEAD $(G rev-parse HEAD)"; echo "8fabc03b3 = $(G rev-parse '8fabc03b3^{commit}')"; echo "git diff --stat 8fabc03b3 HEAD -- cmd internal web/src:"; G diff --stat 8fabc03b3 HEAD -- cmd internal web/src; echo "(end; empty = no product change after 8fabc03b3)"; echo "v0.1.0-alpha.82 = $(G rev-parse 'v0.1.0-alpha.82^{commit}')"; echo "v0.1.0-alpha.81 = $(G rev-parse 'v0.1.0-alpha.81^{commit}')"; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh a > $R/logs/mkov-a.txt 2>&1; echo "mkov a rc=$?"; tail -n 4 $R/logs/mkov-a.txt
