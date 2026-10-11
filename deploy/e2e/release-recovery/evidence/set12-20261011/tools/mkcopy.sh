#!/bin/bash
# set12: run copy NAME = copy a's archive (8fabc03b3) + the set12 harness files of the working tree; the cells run
# from the newest such copy. usage: mkcopy.sh NAME
J=<scratchpad>/set12/tools
export PYTHONDONTWRITEBYTECODE=1
R=/var/tmp/cp-set12-run; L=$R/logs
bash $J/mkov.sh $1 set12_fixture.py build-set12-artifacts.sh set12_trial.py guest_set12_native.py run-set12.sh test_set12_trial.py > $L/mkov-$1.txt 2>&1 || { tail -n 5 $L/mkov-$1.txt; exit 1; }
# the browser script lives under web/tools/browser-inspect; copied into the run copy too (listed with its digest)
REPO='<repo>'
sed 's/\r$//' "$REPO/web/tools/browser-inspect/cold-load-set12.mjs" > $R/harness-$1/web/tools/browser-inspect/cold-load-set12.mjs
sha256sum $R/harness-$1/web/tools/browser-inspect/cold-load-set12.mjs | sed "s#$R/harness-$1/##" >> $R/overlay-$1/files.sha256
tail -n 9 $L/mkov-$1.txt
cd $R/harness-$1/deploy/e2e/release-recovery && python3 -I -c "import ast; [ast.parse(open(f).read()) for f in ('set12_trial.py','guest_set12_native.py','set12_fixture.py','test_set12_trial.py')]; print('set12 files parse')"
bash -n run-set12.sh && bash -n build-set12-artifacts.sh && echo "shell files parse"
SET12_RUN_ROOT=$R python3 -m unittest test_set12_trial test_set11_trial test_set10_trial 2>&1 | tail -n 6
