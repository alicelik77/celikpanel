#!/bin/bash
# set19: the run directory, the repository clone from the bundle, the source check, the harness run copy (git archive of $C).
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
[ -e $R ] && { echo "refusing: $R exists"; exit 2; }
mkdir -p $L $R/host $R/build
echo "$(date -u +%FT%TZ) set19 run directory created" > $R/progress.txt
git clone --quiet /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19.bundle $R/repo
git -C $R/repo fetch --quiet origin 'refs/tags/*:refs/tags/*'
git -C $R/repo checkout --quiet --detach $C
{ echo "source $C tree $(git -C $R/repo rev-parse "$C^{tree}")"
  echo "tag v0.1.0-alpha.90 = $(git -C $R/repo rev-parse 'v0.1.0-alpha.90^{commit}')"
  echo "tag v0.1.0-alpha.91 = $(git -C $R/repo rev-parse 'v0.1.0-alpha.91^{commit}')"
  echo "git log --oneline v0.1.0-alpha.91..$C:"; git -C $R/repo log --oneline "v0.1.0-alpha.91..$C"; echo "(end)"
  echo "git diff --stat v0.1.0-alpha.90 $C -- internal/db/migrations:"; git -C $R/repo diff --stat v0.1.0-alpha.90 $C -- internal/db/migrations; echo "(end; empty = no migration)"
  echo "release policy at $C:"; git -C $R/repo show "$C:deploy/release-sequence-policy"; } > $R/host/source-check.txt
mkdir -p $R/harness
git -C $R/repo archive $C | tar -x -C $R/harness --exclude=deploy/e2e/dns-kill-matrix/evidence --exclude=deploy/e2e/dns-pair-acceptance/evidence --exclude='deploy/e2e/release-recovery/evidence/*'
( cd $R/harness && find . -type f | wc -l ) > $R/host/harness-file-count.txt
{ echo "qemu: $(qemu-system-x86_64 --version | head -n 1)"; echo "kernel (WSL): $(uname -r)"; echo "python: $(python3 --version)"; echo "node: $(node --version)"; echo "/dev/kvm: $(ls -l /dev/kvm)"; /opt/celikpanel-test-toolchains/go1.26.5/go/bin/go version; } > $R/host/hostcheck.txt
echo "$(date -u +%FT%TZ) repository clone, source check and run copy made" >> $R/progress.txt
cat $R/host/source-check.txt | tail -n 20; cat $R/host/harness-file-count.txt
