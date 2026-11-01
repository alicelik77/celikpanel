for f in pairA.out singleR.out singleN.out opsA.out opsR.out opsN.out; do [ -f /var/tmp/a114r4/$f ] && { echo "=== $f"; tail -n ${N:-6} /var/tmp/a114r4/$f | cut -c1-260; }; done
