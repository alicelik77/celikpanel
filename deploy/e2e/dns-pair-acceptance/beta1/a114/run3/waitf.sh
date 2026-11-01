# waitf.sh FILE PATTERN MAXSECONDS : wait until FILE contains PATTERN (bounded), then show the tail
f=/var/tmp/a114r3/$1; t0=$(date +%s)
while [ $(( $(date +%s) - t0 )) -lt $3 ]; do grep -qE "$2" $f 2>/dev/null && break; sleep 15; done
echo "waited $(( $(date +%s) - t0 ))s"; tail -n ${N:-14} $f | cut -c1-300
