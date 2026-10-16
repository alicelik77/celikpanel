#!/bin/bash
# set19: fixture proof and the dry runs of both cells (no guest).
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/set19tools/env.sh
H=$R/harness/deploy/e2e/release-recovery
A=$(cat $R/artifacts.path)
python3 $H/set19_fixture.py prove --artifacts $A > $R/build/prove.json 2> $R/build/prove.stderr.txt; echo "prove rc=$?"
for c in catchup failed; do bash $H/run-set19.sh dry-run set19-$c-debian13 $A s19-dry-$c > $R/build/dry-$c.json 2> $R/build/dry-$c.err; echo "dry $c rc=$?"; done
echo "$(date -u +%FT%TZ) prove and dry runs" >> $R/progress.txt
tail -c 600 $R/build/prove.stderr.txt; tail -c 400 $R/build/dry-catchup.err; python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('schema'), d.get('cell'), d.get('added_steps'))" $R/build/dry-catchup.json
