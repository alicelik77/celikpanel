#!/bin/bash
# set19: run copy b = run copy a (git archive of $C) + the corrected set19_trial.py of f1fe0685c (LF), with its SHA-256 and diff.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
O=$R/overlay-b; mkdir -p $O
f=deploy/e2e/release-recovery/set19_trial.py
cp $R/harness/$f $O/set19_trial.py.run-a
sed 's/\r$//' /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/wt92/$f > $O/incoming.tmp && install -m 0644 $O/incoming.tmp $R/harness/$f && rm -f $O/incoming.tmp
sha256sum $R/harness/$f | sed "s#$R/harness/##" > $O/files.sha256
diff -u --label "a/$f" --label "b/$f" $O/set19_trial.py.run-a $R/harness/$f > $O/harness.diff
echo "$(date -u +%FT%TZ) run copy b: set19_trial.py of f1fe0685c overlaid (lab preparation before the pre-update state)" >> $R/progress.txt
cat $O/files.sha256; wc -l $O/harness.diff
