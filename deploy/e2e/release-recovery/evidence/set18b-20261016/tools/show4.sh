#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-s18b-a/evidence/*/upd1/*/ | tail -1)
ls $d/steps/09-set18-readings $d/steps/08-set18-browser-window
python3 -I - $d <<'PY'
import json, sys, glob
d = sys.argv[1]
st = json.load(open(glob.glob(d + "/steps/09-set18-readings/step.json")[0]))
c = st.get("checks") or {}
print("api:", {k: (v.get("status"), v.get("at")) for k, v in (c.get("api") or {}).items()})
print("setup_status", c.get("setup_status"), "extension", c.get("setup_extension"))
print("ledger", json.dumps(c.get("ledger"))[:400])
print("units", c.get("lab_units"))
print("block_log", c.get("lab_block_log"))
print("renewal", c.get("lab_renewal"))
w = json.load(open(glob.glob(d + "/steps/08-set18-browser-window/step.json")[0]))
wc = w.get("checks") or {}
print("pin_after_window", wc.get("pin_after_the_window"), wc.get("pin_after_the_window_error"))
print("prep_count", wc.get("prep_count"))
PY
sed -n 1,60p $d/steps/09-set18-readings/native-reading.txt | cut -c1-200
