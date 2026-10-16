#!/bin/bash
source <redacted>
name=${1:-c}
bash $J/mkov.sh $name deploy/e2e/release-recovery/set18b_fixture.py deploy/e2e/release-recovery/build-set18b-artifacts.sh deploy/e2e/release-recovery/set18b_trial.py deploy/e2e/release-recovery/run-set18b.sh web/tools/browser-inspect/cold-load-set18b.mjs > $L/mkov-$name.txt 2>&1; echo "mkov $name rc=$?"; tail -n 6 $L/mkov-$name.txt
H=$R/harness-$name/deploy/e2e/release-recovery
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=$R
{ for f in set18b_trial.py set18b_fixture.py set18_trial.py set18_fixture.py guest_set18_lab_dns.py; do python3 -I -B -m py_compile "$H/$f" && echo "compiled $f"; done
  bash -n $H/run-set18b.sh && bash -n $H/build-set18b-artifacts.sh && echo "shell syntax ok"
  node --check $R/harness-$name/web/tools/browser-inspect/cold-load-set18b.mjs && echo "tool syntax ok"
  python3 -I -B $J/snitest.py $H > $R/build/snitest-$name.json; echo "sni test rc=$?"; cat $R/build/snitest-$name.json
  if [ -s $R/artifacts.path ]; then
    python3 -I -B $H/set18b_fixture.py prove --artifacts "$(cat $R/artifacts.path)" > $R/build/prove-$name.json 2> $R/build/prove-$name.stderr.txt; echo "prove rc=$?"
    python3 -I -B $H/set18b_trial.py plan --cell set18-debian13 --artifacts "$(cat $R/artifacts.path)" --work-root /var/tmp/cp-release-drill-s18b-a --dry-run > $R/build/dry-$name.json 2> $R/build/dry-$name.err; echo "dry rc=$?"
  fi; } 2>&1 | tee $L/suite-$name.txt
