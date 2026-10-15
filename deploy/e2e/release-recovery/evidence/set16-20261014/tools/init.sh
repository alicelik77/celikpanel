#!/bin/bash
# set16: the run directory, the host record, the source check, run copy b (git archive of $C + set16 build files).
source <redacted>
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set16 run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
{ echo "HEAD $(G rev-parse HEAD)"; echo "source $C tree $(G rev-parse "$C^{tree}")"; echo "git diff --stat $C HEAD -- cmd internal web/src:"; G diff --stat $C HEAD -- cmd internal web/src; echo "(end; empty = no product change after $C)"; echo "git status --short -- cmd internal web/src deploy/release-sequence-policy:"; timeout 120 git -c safe.directory="*" -C "$REPO" status --short -- cmd internal web/src deploy/release-sequence-policy || echo "(git status rc=$?)"; echo "(end; empty = no uncommitted product change)"; echo "tag v0.1.0-alpha.84 = $(G rev-parse 'v0.1.0-alpha.84^{commit}')"; echo "git diff --stat v0.1.0-alpha.84 $C -- internal/db/migrations:"; G diff --stat v0.1.0-alpha.84 $C -- internal/db/migrations; echo "(end)"; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh b deploy/e2e/release-recovery/set16_fixture.py deploy/e2e/release-recovery/build-set16-artifacts.sh > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; tail -n 4 $L/mkov-b.txt
