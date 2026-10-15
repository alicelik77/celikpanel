#!/bin/bash
# set17: the offline suites in a given harness directory, isolated Python, no bytecode.
H=${1:-'/mnt/c/CELIKBROS PROJECTS/celikpanel/deploy/e2e/release-recovery'}
cd "$H" || exit 2
for t in test_set16_trial.py test_set15_trial.py; do
  timeout 600 python3 -I -B -m unittest discover -s "$H" -t "$H" -p "$t" -v 2>&1 | grep -E "^(Ran|OK|FAILED|ERROR|FAIL|Traceback|[A-Za-z]+Error)" | head -20
done
for f in set17_trial.py set17_fixture.py; do python3 -I -B -m py_compile "$H/$f" && echo "compiled $f"; done
bash -n "$H/run-set17.sh" && bash -n "$H/build-set17-artifacts.sh" && echo "shell syntax ok"
node --check "$H/../../../web/tools/browser-inspect/cold-load-set17.mjs" && echo "tool syntax ok"
