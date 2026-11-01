#!/bin/bash
cd /var/tmp/a114r2
touch pairA.out opsA.out archB.out opsB.out cd.out
tail -n0 -F pairA.out opsA.out archB.out opsB.out cd.out 2>/dev/null | grep --line-buffered -E '^== |^FAIL|E1 backup|Traceback|bound reached|done [0-9]' | cut -c1-300
