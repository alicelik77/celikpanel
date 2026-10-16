#!/bin/bash
for s in none localhost panel.set18.test; do
  if [ $s = none ]; then a="-noservername"; else a="-servername $s"; fi
  echo "== SNI $s"; echo | timeout 10 openssl s_client -connect 127.0.0.1:19320 $a 2>/dev/null | openssl x509 -noout -subject -issuer -fingerprint -sha256 2>&1
done
