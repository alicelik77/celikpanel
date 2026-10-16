#!/bin/bash
# set18b: the run directory, the host record, the source check, run copy b (git archive of $C + set18 build files).
source <redacted>
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set18b run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
{ echo "HEAD $(G rev-parse HEAD)"; echo "source $C tree $(G rev-parse "$C^{tree}")";
  echo "tag v0.1.0-alpha.86 = $(G rev-parse 'v0.1.0-alpha.86^{commit}')";
  echo "git log --oneline v0.1.0-alpha.86..$C:"; G log --oneline "v0.1.0-alpha.86..$C"; echo "(end)";
  echo "git diff --stat v0.1.0-alpha.86 $C -- cmd internal web/src:"; G diff --stat v0.1.0-alpha.86 $C -- cmd internal web/src; echo "(end)";
  echo "git diff --stat v0.1.0-alpha.86 $C -- internal/db/migrations:"; G diff --stat v0.1.0-alpha.86 $C -- internal/db/migrations; echo "(end; empty = no migration)";
  echo "git status --short -- cmd internal web/src deploy/release-sequence-policy update.sh install.sh:"; timeout 120 git -c safe.directory="*" -C "$REPO" status --short -- cmd internal web/src deploy/release-sequence-policy update.sh install.sh || echo "(git status rc=$?)"; echo "(end; empty = no uncommitted product change)";
  echo "release policy at $C:"; G show "$C:deploy/release-sequence-policy"; } > $R/host/source-check.txt
cat $R/host/source-check.txt | tail -n 30
bash $J/mkov.sh b deploy/e2e/release-recovery/set18b_fixture.py deploy/e2e/release-recovery/build-set18b-artifacts.sh > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; tail -n 4 $L/mkov-b.txt
