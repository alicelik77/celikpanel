#!/bin/bash
L=/var/tmp/cp-set11-run/logs
cat $L/suites.out
for c in a e; do echo "== $c"; tail -n 3 $L/suite-$c.txt; cat $L/suite-$c-notok.txt; done
cmp $L/suite-a-notok.txt $L/suite-e-notok.txt && echo "the not-OK tests of a and e are the same by name"
grep -c "^test_set11\|set11" $L/suite-e.txt
grep -E "^Ran " $L/suite-a.txt $L/suite-e.txt
