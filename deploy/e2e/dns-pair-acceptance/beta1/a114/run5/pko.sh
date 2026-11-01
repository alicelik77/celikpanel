t=0; until grep -qE "OPS-EXIT|FAILFAST" /var/tmp/a114r5/${2:-opsA.out} || [ $t -ge ${3:-160} ]; do sleep 10; t=$((t+10)); done
grep -E "^== |^-- |^PASS|^FAIL|DELSTATUS|FAILFAST|OPS-EXIT|@192|^ +[0-9:]+ \+[0-9]+s|delete-|^[0-9]{3} " /var/tmp/a114r5/${2:-opsA.out} | tail -n ${1:-25} | cut -c1-420
