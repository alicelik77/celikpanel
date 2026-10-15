#!/bin/bash
# set17b: the run directory, the host record, the source check, run copy b (git archive of $C + set17 build files).
source <redacted>
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set17b run directory created" > $R/progress.txt
bash $J/hostcheck.sh > $R/host/hostcheck-before.txt 2>&1
{ echo "HEAD $(G rev-parse HEAD)"; echo "source $C tree $(G rev-parse "$C^{tree}")";
  echo "set17's source 313f3eea2c9b89251588648b09484cd34b7aecb1";
  echo "git diff --stat 313f3eea2 $C -- cmd internal web/src deploy/release-sequence-policy update.sh install.sh:"; G diff --stat 313f3eea2 $C -- cmd internal web/src deploy/release-sequence-policy update.sh install.sh; echo "(end)";
  echo "git log --oneline 313f3eea2..$C:"; G log --oneline "313f3eea2..$C"; echo "(end)";
  echo "git status --short -- cmd internal web/src deploy/release-sequence-policy update.sh:"; timeout 120 git -c safe.directory="*" -C "$REPO" status --short -- cmd internal web/src deploy/release-sequence-policy update.sh || echo "(git status rc=$?)"; echo "(end; empty = no uncommitted product change)";
  echo "release policy at $C:"; G show "$C:deploy/release-sequence-policy";
  echo "tag v0.1.0-alpha.85 = $(G rev-parse 'v0.1.0-alpha.85^{commit}')";
  echo "git diff --stat v0.1.0-alpha.85 $C -- internal/db/migrations:"; G diff --stat v0.1.0-alpha.85 $C -- internal/db/migrations; echo "(end)";
  echo "update.sh checkpoint lines at $C:"; G show "$C:update.sh" | grep -n 'CELIKPANEL_UPDATE_CHECKPOINT database_verified_before_start';
  echo "update.sh checkpoint lines at v0.1.0-alpha.85:"; G show "v0.1.0-alpha.85:update.sh" | grep -n 'CELIKPANEL_UPDATE_CHECKPOINT database_verified_before_start'; } > $R/host/source-check.txt
cat $R/host/source-check.txt
bash $J/mkov.sh b deploy/e2e/release-recovery/set17_fixture.py deploy/e2e/release-recovery/build-set17-artifacts.sh > $L/mkov-b.txt 2>&1; echo "mkov b rc=$?"; tail -n 4 $L/mkov-b.txt
