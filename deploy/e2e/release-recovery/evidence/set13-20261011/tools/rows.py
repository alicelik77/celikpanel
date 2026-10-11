"""set13: the vhost/pool ledger rows of one domain in every read-ledger native of the named steps, and the Domains-list
rows the driver recorded (calls). usage: rows.py RUN_DIR DOMAIN_ID STEP_GLOB..."""
import glob
import json
import os
import sys

run, domain_id, patterns = sys.argv[1], int(sys.argv[2]), sys.argv[3:]
for pattern in patterns:
    for step in sorted(glob.glob(os.path.join(run, "steps", pattern))):
        print("==", os.path.basename(step))
        for f in sorted(glob.glob(os.path.join(step, "native", "*.json"))):
            d = json.load(open(f))
            rows = ((d.get("ledger") or {}).get("managed_site_files") or [])
            mine = [r for r in rows if r.get("domain_id") == domain_id]
            if mine:
                print(" ", os.path.basename(f))
                for r in mine:
                    print("    ", {k: r.get(k) for k in ("id", "kind", "state", "state_reason", "decision", "observed_at", "file_sha256")})
        sec = os.path.join(step, "section.json")
        if os.path.exists(sec):
            for c in json.load(open(sec)).get("calls") or []:
                if c.get("request", {}).get("path") == "/api/v1/domains":
                    rows = [r for r in (c.get("json") or []) if isinstance(r, dict) and r.get("id") == domain_id]
                    print("  list", c.get("at"), c.get("label")[:60], [r.get("site_config") for r in rows])
                elif "site-config" in str(c.get("request", {}).get("path")):
                    j = c.get("json") or {}
                    print("  call", c.get("at"), c.get("status"), str(c.get("label"))[:70], {k: j.get(k) for k in ("state", "overall_state", "code", "restore", "service")})
