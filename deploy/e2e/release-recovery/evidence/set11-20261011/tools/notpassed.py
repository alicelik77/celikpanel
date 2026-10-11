#!/usr/bin/env python3
"""set11: every step whose verdict is not passed/observed/skipped and every section check that is not true, from the
staged driver records, each with its class: product (an expectation measured and not held), not-measured, harness
(a check of the harness that measured nothing: the two watchers' sub-millisecond read order, copies c-e), or aborted
(debian13/run-a, stopped after a harness error)."""
import glob, json, os, sys
E = sys.argv[1]
HARNESS_CHECK = "the file was there for every GET"
out = []
for run in sorted(glob.glob(E + "/*/*/driver")):
    rel = os.path.relpath(os.path.dirname(run), E).replace(os.sep, "/")
    aborted = os.path.exists(os.path.join(os.path.dirname(run), "host", "driver-not-finalized.txt"))
    for f in sorted(glob.glob(run + "/steps/*/step.json")):
        s = json.load(open(f, encoding="utf-8"))
        if s.get("verdict") in ("passed", "observed", "skipped"):
            continue
        name = s.get("name")
        reason = (s.get("reason") or "")
        cls = ("aborted" if aborted else "harness" if HARNESS_CHECK in reason and "the page and the Domains row" not in reason
               else "product" if s.get("verdict") == "failed" else "not-measured")
        out.append(f"{rel}\tstep\t{name}\t{s.get('verdict')}\t{cls}\t{reason[:300]}")
    for f in sorted(glob.glob(run + "/steps/*/section.json")):
        s = json.load(open(f, encoding="utf-8"))
        for c in s.get("checks") or []:
            if c["ok"] is True:
                continue
            cls = ("aborted" if aborted else "harness" if HARNESS_CHECK in c["name"] else
                   "product" if c["ok"] is False else "not-measured")
            out.append(f"{rel}\tcheck\t{os.path.basename(os.path.dirname(f))}\t{c['ok']}\t{cls}\t{c['name'][:220]}")
print("run\tkind\tname\tverdict\tclass\tdetail")
print("\n".join(out))
