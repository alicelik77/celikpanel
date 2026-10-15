#!/bin/bash
# set16: the offline suites in a given harness directory (default: the working tree), isolated Python, no bytecode.
H=${1:-'/mnt/c/CELIKBROS PROJECTS/celikpanel/deploy/e2e/release-recovery'}
cd "$H" || exit 2
for t in test_set16_trial.py test_set15_trial.py; do
  timeout 600 python3 -I -B -m unittest discover -s "$H" -t "$H" -p "$t" -v 2>&1 | grep -E "^(test_|Ran|OK|FAILED|ERROR|FAIL|Traceback|  File|[A-Za-z]+Error)" | head -40
done
for f in set16_trial.py set16_fixture.py; do python3 -I -B -m py_compile "$H/$f" && echo "compiled $f"; done
bash -n "$H/run-set16.sh" && bash -n "$H/build-set16-artifacts.sh" && echo "shell syntax ok"
