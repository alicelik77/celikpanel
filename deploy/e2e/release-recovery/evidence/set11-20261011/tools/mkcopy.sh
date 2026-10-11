#!/bin/bash
# set11: run copy NAME = copy a's archive (ead69cdcc) + the set11 harness files of the working tree; the cells run
# from the newest such copy. usage: mkcopy.sh NAME
J=<scratchpad>/set11/tools
export PYTHONDONTWRITEBYTECODE=1
R=/var/tmp/cp-set11-run; L=$R/logs
bash $J/mkov.sh $1 set11_trial.py guest_set11_native.py run-set11.sh test_set11_trial.py > $L/mkov-$1.txt 2>&1 || { tail -n 5 $L/mkov-$1.txt; exit 1; }
tail -n 8 $L/mkov-$1.txt
cd $R/harness-$1 && python3 -I -c "import ast; [ast.parse(open('deploy/e2e/release-recovery/'+f).read()) for f in ('set11_trial.py','guest_set11_native.py','test_set11_trial.py')]; print('set11 files parse')"
bash -n $R/harness-$1/deploy/e2e/release-recovery/run-set11.sh && echo "run-set11.sh parses"
cd $R/harness-$1/deploy/e2e/release-recovery && python3 -m unittest test_set11_trial test_set10_trial 2>&1 | tail -n 6
