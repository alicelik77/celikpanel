#!/bin/bash
source <redacted>
ART=$(cat $R/artifacts-fresh.path); BD=$(dirname "$ART")
python3 -I -c 'import json,sys;d=json.load(open(sys.argv[1]));b=d["baseline"];print({k:b.get(k) for k in ("version","sequence","commit","sha256","dist_json")})' "$ART"
c=$(python3 -I -c 'import json,sys;print(json.load(open(sys.argv[1]))["baseline"]["commit"])' "$ART")
git -c safe.directory='*' -C $BD/repo diff --name-status $(G rev-parse "$C^{commit}") $c
