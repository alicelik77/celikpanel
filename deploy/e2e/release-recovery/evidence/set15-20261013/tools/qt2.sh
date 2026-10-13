#!/bin/bash
set -u
T=$(mktemp -d /var/tmp/cp-set15-run/quick.XXXX)
cd '<redacted> PROJECTS/celikpanel/deploy/e2e/release-recovery'
for f in *.py *.sh; do sed 's/\r$//' "$f" > "$T/$f"; done
cd $T && SET12_RUN_ROOT=$T/root PYTHONDONTWRITEBYTECODE=1 timeout 300 python3 -m unittest $* 2>&1 | grep -vE '^\s*$' | tail -n 40 | cut -c1-250
cd / && rm -rf -- "$T"
