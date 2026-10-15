#!/bin/bash
# set17: the run directory, the host record, the source check, run copy b (git archive of $C + set17 build files).
source <redacted>
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set17 run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
{ echo "HEAD $(G rev-parse HEAD)"; echo "source $C tree $(G rev-parse "$C^{tree}")";
  echo "last product commit $(G rev-parse 21f07e7a8)";
  echo "git diff --stat 21f07e7a8 $C -- cmd internal web:"; G diff --stat 21f07e7a8 $C -- cmd internal web; echo "(end; empty = $C changes no product file)";
  echo "git diff --name-only 21f07e7a8 $C:"; G diff --name-only 21f07e7a8 $C; echo "(end)";
  echo "git diff --stat $C HEAD -- cmd internal web deploy/release-sequence-policy:"; G diff --stat $C HEAD -- cmd internal web deploy/release-sequence-policy; echo "(end; empty = no product change after $C)";
  echo "git status --short -- cmd internal web/src deploy/release-sequence-policy:"; timeout 120 git -c safe.directory="*" -C "$REPO" status --short -- cmd internal web/src deploy/release-sequence-policy || echo "(git status rc=$?)"; echo "(end; empty = no uncommitted product change)";
  echo "release policy at $C:"; G show "$C:deploy/release-sequence-policy";
  echo "tag v0.1.0-alpha.85 = $(G rev-parse 'v0.1.0-alpha.85^{commit}')";
  echo "git diff --stat v0.1.0-alpha.85 $C -- internal/db/migrations:"; G diff --stat v0.1.0-alpha.85 $C -- internal/db/migrations; echo "(end)";
  echo "git diff --stat v0.1.0-alpha.85 $C -- cmd/panel/main.go:"; G diff --stat v0.1.0-alpha.85 $C -- cmd/panel/main.go; echo "(end)"; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh b deploy/e2e/release-recovery/set17_fixture.py deploy/e2e/release-recovery/build-set17-artifacts.sh > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; tail -n 4 $L/mkov-b.txt
