#!/bin/bash
source <redacted>
grep -v '^ \|^Traceback\|^Exception\|^----\|BrokenPipe' $L/suite-${1:-c}.txt | head -20
head -c 700 $R/build/dry-${1:-c}.json; echo; tail -n 3 $R/build/dry-${1:-c}.err
head -c 500 $R/build/prove-${1:-c}.json; echo; tail -n 3 $R/build/prove-${1:-c}.stderr.txt
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print({k:(d[k]["version"],d[k]["commit"][:12],d[k]["sha256"][:16]) for k in ("baseline","good","defective")})' "$(cat $R/artifacts.path)"
