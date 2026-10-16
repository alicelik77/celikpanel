#!/bin/bash
# set18b: job-build.sh named set18's build log in its last grep (fixed in the tool after the run), so artifacts.path
# was empty; the coordinator writes it from build-set18b.out and runs the suite's last two checks on run copy c.
source <redacted>
name=${1:-c}
[ -s $R/artifacts.path ] || { grep -m1 'upd1-artifacts.json' $L/build-set18b.out > $R/artifacts.path; echo "$(date -u +%FT%TZ) coordinator wrote artifacts.path from build-set18b.out (job-build.sh grep named build-set18.out)" >> $R/progress.txt; }
cat $R/artifacts.path
H=$R/harness-$name/deploy/e2e/release-recovery
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
{ python3 -I -B $H/set18b_fixture.py prove --artifacts "$(cat $R/artifacts.path)" > $R/build/prove-$name.json 2> $R/build/prove-$name.stderr.txt; echo "prove rc=$?"
  python3 -I -B $H/set18b_trial.py plan --cell set18-debian13 --artifacts "$(cat $R/artifacts.path)" --work-root /var/tmp/cp-release-drill-s18b-a --dry-run > $R/build/dry-$name.json 2> $R/build/dry-$name.err; echo "dry rc=$?"; } 2>&1 | tee -a $L/suite-$name.txt
