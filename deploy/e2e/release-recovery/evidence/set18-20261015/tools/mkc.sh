#!/bin/bash
source <redacted>
name=${1:-c}
bash $J/mkov.sh $name deploy/e2e/release-recovery/set18_fixture.py deploy/e2e/release-recovery/build-set18-artifacts.sh deploy/e2e/release-recovery/set18_trial.py deploy/e2e/release-recovery/run-set18.sh deploy/e2e/release-recovery/guest_set18_lab_dns.py web/tools/browser-inspect/cold-load-set18.mjs > $L/mkov-$name.txt 2>&1; echo "mkov $name rc=$?"; tail -n 7 $L/mkov-$name.txt
H=$R/harness-$name/deploy/e2e/release-recovery
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
{ for f in set18_trial.py set18_fixture.py guest_set18_lab_dns.py; do python3 -I -B -m py_compile "$H/$f" && echo "compiled $f"; done
  bash -n $H/run-set18.sh && bash -n $H/build-set18-artifacts.sh && echo "shell syntax ok"
  node --check $R/harness-$name/web/tools/browser-inspect/cold-load-set18.mjs && echo "tool syntax ok"
  python3 -I -B $H/set18_fixture.py prove --artifacts "$(cat $R/artifacts.path)" > $R/build/prove.json 2> $R/build/prove.stderr.txt; echo "prove rc=$?"
  python3 -I -B $H/set18_trial.py plan --cell set18-debian13 --artifacts "$(cat $R/artifacts.path)" --work-root /var/tmp/cp-release-drill-s18-a --dry-run > $R/build/dry-$name.json 2> $R/build/dry-$name.err; echo "dry rc=$?"; } 2>&1 | tee $L/suite-$name.txt
tail -n 5 $R/build/dry-$name.err; head -c 600 $R/build/dry-$name.json; echo; head -c 400 $R/build/prove.json; tail -n 3 $R/build/prove.stderr.txt
