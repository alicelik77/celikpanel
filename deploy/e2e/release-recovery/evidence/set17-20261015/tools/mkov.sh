#!/bin/bash
# set17: a run copy = git archive $C without retained evidence folders + exactly the listed harness files from the
# working tree (CRLF -> LF), each with its SHA-256, and their diff against $C. usage: mkov.sh NAME [FILE...]
set -euo pipefail
source <redacted>
name=$1; shift
H=$R/harness-$name
O=$R/overlay-$name
[ -e $H ] && { echo "refusing: $H exists"; exit 2; }
mkdir -p $H $O
G archive $C | tar -x -C $H --exclude=deploy/e2e/dns-kill-matrix/evidence --exclude=deploy/e2e/dns-pair-acceptance/evidence --exclude='deploy/e2e/release-recovery/evidence/set*' --exclude='deploy/e2e/release-recovery/evidence/roottests*' --exclude='deploy/e2e/release-recovery/evidence/upd[2-9]*' --exclude='deploy/e2e/release-recovery/evidence/upd1[0-9]*'
( cd $H && find . -type f | wc -l ) > $O/pristine-file-count.txt
: > $O/files.sha256; : > $O/harness.diff
for f in "$@"; do
  mode=0644; case $f in *.sh) mode=0755;; esac
  mkdir -p "$(dirname "$H/$f")"
  sed 's/\r$//' "$REPO/$f" > "$O/incoming.tmp"; install -m $mode "$O/incoming.tmp" "$H/$f"
  sha256sum "$H/$f" | sed "s#$H/##" >> $O/files.sha256
  if G cat-file -e "$C:$f" 2>/dev/null; then
    G show "$C:$f" | diff -u --label "a/$f" --label "b/$f" - "$H/$f" >> $O/harness.diff || true
  else
    diff -u --label /dev/null --label "b/$f" /dev/null "$H/$f" >> $O/harness.diff || true
  fi
done
rm -f "$O/incoming.tmp"
echo "run copy $name of $C: $(cat $O/pristine-file-count.txt) pristine files; overlay:"; cat $O/files.sha256
echo "$(date -u +%FT%TZ) made run copy $name" >> $R/progress.txt
