#!/usr/bin/env python3
"""set13: every step whose verdict is not passed/observed/skipped and every section check that is not true, from the
staged driver records, each with its class: harness (a harness error or an unmet precondition named in the README:
debian13/fresh-a C4 and C5, ubuntu/fresh-a C4), product (an expectation measured and not held), not-measured.
usage: notpassed.py EVIDENCE"""
import glob
import json
import os
import sys

E = sys.argv[1]
HARNESS = {("debian13/fresh-a", "C4-pool-not-read"): "harness: precondition (C3 before C4 left the vhost row foreign/nginx_refused); measured in debian13/fresh-b",
           ("debian13/fresh-a", "C5-app-unit"): "harness: the harness's own systemd-analyze while hidden (e); measured in debian13/fresh-b",
           ("ubuntu/fresh-a", "C4-pool-not-read"): "harness: precondition (no answered read after C2's refused take and the lab put-back); measured in ubuntu/fresh-b"}
out = []
for run in sorted(glob.glob(E + "/*/*/driver")):
    rel = os.path.relpath(os.path.dirname(run), E).replace(os.sep, "/")
    for f in sorted(glob.glob(run + "/steps/*/step.json")):
        s = json.load(open(f, encoding="utf-8"))
        if s.get("verdict") in ("passed", "observed", "skipped"):
            continue
        name, reason = s.get("name"), (s.get("reason") or "")
        cls = HARNESS.get((rel, name)) or ("product" if s.get("verdict") == "failed" else "not-measured")
        out.append(f"{rel}\tstep\t{name}\t{s.get('verdict')}\t{cls}\t{reason[:300]}")
    for f in sorted(glob.glob(run + "/steps/*/section.json")):
        s = json.load(open(f, encoding="utf-8"))
        for c in s.get("checks") or []:
            if c["ok"] is True:
                continue
            cls = HARNESS.get((rel, s.get("section"))) or ("product" if c["ok"] is False else "not-measured")
            out.append(f"{rel}\tcheck\t{os.path.basename(os.path.dirname(f))}\t{c['ok']}\t{cls}\t{c['name'][:220]}")
open(os.path.join(E, "checks-not-passed.txt"), "w", encoding="utf-8", newline="\n").write(
    "run\tkind\tname\tverdict\tclass\tdetail\n" + "\n".join(out) + "\n")
print("\n".join(out))
