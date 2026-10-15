#!/bin/bash
# set16: the dist folders this run's builds wrote, with each dist.json's identity and every archive's SHA-256 (reads only).
for d in /var/tmp/cp-pair-accept/dist/*/; do
  echo "== $d"
  ls "$d" | grep -v product-web-src | head -n 8
  python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]));print({k:d.get(k) for k in ("sha256","version","commit","license_mode","archive","release")})' "$d/dist.json" 2>&1 | cut -c1-400
  for a in "$d"*.tar.gz; do [ -f "$a" ] && echo "sha256 $(sha256sum "$a" | cut -c1-64) $(basename "$a")"; done
done
