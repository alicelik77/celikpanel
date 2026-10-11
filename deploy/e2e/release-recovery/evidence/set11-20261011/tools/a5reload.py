#!/usr/bin/env python3
"""set11: for A5 of each staged run, the nginx reload lines of the journal window relative to the owner's own reload and
to the watch start (the read itself), and the events in CelikPanel's include directory during the read."""
import glob, json, os, sys
from datetime import datetime
E = sys.argv[1]
for step in sorted(glob.glob(os.path.join(E, "*", "run-*", "driver", "steps", "*-a5-include-back"))):
    natives = sorted(glob.glob(os.path.join(step, "native", "*.json")))
    pick = lambda part: next((json.load(open(f)) for f in natives if part in os.path.basename(f)), {})
    owner, start, stop = pick("owner-a5-add-managed-include"), pick("a5-watch-start"), pick("a5-watch-stop")
    journal = pick("a5-journal")
    lines = [l for l in (journal.get("by_unit") or {}).get("nginx.service", []) if "Reloaded" in l]
    t = lambda l: datetime.fromisoformat(l.split()[0]).timestamp()
    after = [l for l in lines if t(l) >= float(start.get("epoch") or 0)]
    managed = [r for r in stop.get("fs", []) if "celikpanel-managed.d" in str(r.get("dir"))]
    print(os.path.relpath(step, E), "| owner reload ended", owner.get("at"), "| watch start", start.get("at"),
          "| Reloaded lines in window:", len(lines), "after the watch start:", len(after), "| managed-dir events during the read:", len(managed))
