#!/bin/bash
# waitfor.sh SECONDS FILE PATTERN
timeout "$1" bash -c "until grep -q -- \"$3\" /var/tmp/a114r1/$2 2>/dev/null; do sleep 10; done"; echo "rc=$?"; date -u +%T
