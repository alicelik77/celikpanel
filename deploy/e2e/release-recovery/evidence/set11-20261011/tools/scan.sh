#!/bin/bash
# set11: the secret scan over the staged evidence (the lab key files and raw records are read on this host)
R=/var/tmp/cp-set11-run
E="<repo>/deploy/e2e/release-recovery/evidence/$(cat $R/evidence-name.txt)"
J=<scratchpad>/set11/tools
SET8_SCANVALUES=$R/scanvalues SET6_HARNESS=$R/harness-f/deploy/e2e/release-recovery python3 -I -B $J/secretscan.py "$E" /var/tmp/cp-release-drill-s11-* > $R/secret-scan.txt 2> $R/secret-scan.err; echo "scan rc=$?"
tail -n 3 $R/secret-scan.err
grep -v "^  /var/tmp" $R/secret-scan.txt | cut -c1-260
cp $R/secret-scan.txt "$E/secret-scan.txt"
