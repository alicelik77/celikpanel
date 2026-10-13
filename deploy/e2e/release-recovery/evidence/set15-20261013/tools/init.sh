#!/bin/bash
# set15: the run directory, the host record and run copy a (pristine git archive of 1636b73f0).
source <redacted>
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set15 run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
{ echo "HEAD $(G rev-parse HEAD)"; echo "1636b73f0 = $(G rev-parse "$C^{commit}") tree $(G rev-parse "$C^{tree}")"; echo "git diff --stat $C HEAD -- cmd internal web/src:"; G diff --stat $C HEAD -- cmd internal web/src; echo "(end; empty = no product change after $C in the commit graph)"; echo "git status --short -- cmd internal web/src:"; timeout 120 git -c safe.directory="*" -C "$REPO" status --short -- cmd internal web/src || echo "(git status rc=$?; the host Git records it instead)"; echo "(end; empty = no uncommitted product change)"; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh a > $L/mkov-a.txt 2>&1; echo "mkov a rc=$?"; tail -n 6 $L/mkov-a.txt
