#!/bin/bash
# set19: the dry runs of both cells from run copy c, and the catch-up preparation text it will run.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
cd $R/harness/deploy/e2e/release-recovery || exit 2
python3 -c 'import set19_trial as t; print(t.PREPARATIONS["catchup"])' > $R/build/prep-catchup-c.txt
python3 -c 'import set19_trial as t; print(t.PREPARATIONS["failed"])' > $R/build/prep-failed-c.txt
for c in catchup failed; do bash run-set19.sh dry-run set19-$c-debian13 $(cat $R/artifacts.path) s19-dry-c-$c > $R/build/dry-c-$c.json 2> $R/build/dry-c-$c.err; echo "dry c $c rc=$?"; done
echo "$(date -u +%FT%TZ) run copy c dry runs" >> $R/progress.txt
head -c 700 $R/build/prep-catchup-c.txt
