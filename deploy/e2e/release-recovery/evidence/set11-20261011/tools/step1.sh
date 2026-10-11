#!/bin/bash
# set11: the run directory, a read-only host check, and run copy a (the pristine archive of ead69cdcc).
R=/var/tmp/cp-set11-run
T=<scratchpad>/set11/tools
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $R/logs $R/build $R/host
echo "$(date -u +%FT%TZ) set11 run directory created" >> $R/progress.txt
bash $T/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
sha256sum /var/tmp/cp-pair-accept/dist/a0beb7263d1f4ca72258f6b306f9111ba4e2a334-acceptance-license/*.tar.gz >> $R/host/hostcheck-before.txt 2>&1
ls -la /var/tmp/cp-pair-accept/dist/a0beb7263d1f4ca72258f6b306f9111ba4e2a334-acceptance-license/ >> $R/host/hostcheck-before.txt 2>&1
G() { git -c safe.directory='*' -C '<repo>' "$@"; }
{ echo "== commits"; G rev-parse ead69cdcc 'ead69cdcc^{tree}' cd46ca595 HEAD
  echo "== git diff --stat cd46ca595 ead69cdcc -- cmd internal web/src"; G diff --stat cd46ca595 ead69cdcc -- cmd internal web/src
  echo "== git diff --stat ead69cdcc HEAD -- cmd internal web/src (empty = no later product change)"; G diff --stat ead69cdcc HEAD -- cmd internal web/src
  echo "== end"; } > $R/host/source-check.txt 2>&1
bash $T/mkov.sh a > $R/logs/mkov-a.txt 2>&1; echo "mkov a rc=$?"; head -n 8 $R/logs/mkov-a.txt
tail -n 6 $R/host/source-check.txt; tail -n 12 $R/host/hostcheck-before.txt
