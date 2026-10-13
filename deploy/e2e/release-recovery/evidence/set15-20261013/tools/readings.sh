#!/bin/bash
# set15: print a lab's set15 readings (section.json checks) and installed pins. usage: readings.sh LAB NODE
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/$2/upd1/*/ | tail -1)
for s in $(ls -d $d/steps/*set15-readings* 2>/dev/null); do
  echo "## $s"
  python3 -I - "$s/step.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
c=d.get("checks") or d; print("verdict", d.get("verdict"), d.get("reason"))
print(json.dumps({k:c.get(k) for k in ("fields","installed_pins")}, indent=1)[:3000])
l=c.get("ledger") or {}
print("ledger", l.get("verdict"), l.get("schema_version"), l.get("ledger_rows"), json.dumps(l.get("facts")))
for p,v in (c.get("api") or {}).items(): print(p, v.get("status"), json.dumps(v.get("body"))[:600])
PY
done
