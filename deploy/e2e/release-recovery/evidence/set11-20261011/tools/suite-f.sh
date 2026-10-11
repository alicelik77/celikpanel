#!/bin/bash
bash <scratchpad>/set11/tools/suite.sh f
L=/var/tmp/cp-set11-run/logs
cmp $L/suite-a-notok.txt $L/suite-f-notok.txt && echo "copy f: the not-OK tests are those of copy a by name"
grep -E "^Ran " $L/suite-f.txt
