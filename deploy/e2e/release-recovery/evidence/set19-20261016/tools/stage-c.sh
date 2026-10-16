#!/bin/bash
# set19: stage run copy c's overlay and the preparation texts it ran.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
E=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/wt92/deploy/e2e/release-recovery/evidence/set19-20261016
mkdir -p $E/harness-run-copy/c
cp $R/overlay-c/files.sha256 $R/overlay-c/harness.diff $E/harness-run-copy/c/
cp $R/build/prep-catchup-c.txt $R/build/prep-failed-c.txt $E/build/
cp $R/progress.txt $R/neighbour-gate.txt $E/host/
ls $E/harness-run-copy/*
