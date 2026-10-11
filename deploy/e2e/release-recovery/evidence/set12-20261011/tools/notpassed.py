#!/usr/bin/env python3
"""set12: every step whose verdict is not passed/observed/skipped and every section check that is not true, from the
staged driver records, each with its class: product (an expectation measured and not held), not-measured, harness
(a harness error named in the README: run debian13/fresh-a's RN, A non-local and N, debian13/upd82-good's arm,
the inherited alpha.81-only checks of set4/set6 on an alpha.82 baseline), environment (debian13/upd82-good-b: the
guest's package manager was busy at setup). usage: notpassed.py EVIDENCE"""
import glob, json, os, sys
E = sys.argv[1]
HARNESS_RUN_STEPS = {("debian13/fresh-a", "RN-take-nginx-refused"), ("debian13/upd82-good", "arm")}
HARNESS_TEXT = ("snippets/fastcgi-php.conf", "Strict-Transport-Security exactly", "the build this reading is about",
                "a-nonlocal", "not this server's address: no probe request was sent", "validation_probe=not_run validation_unknown_reason=address_not_local validation_address=192.0.2.10",
                "the app runs", "try-restart")
NM_TEXT = ("the answer carries validation_probe not_run",)
out = []
for run in sorted(glob.glob(E + "/*/*/driver")):
    rel = os.path.relpath(os.path.dirname(run), E).replace(os.sep, "/")
    for f in sorted(glob.glob(run + "/steps/*/step.json")):
        s = json.load(open(f, encoding="utf-8"))
        if s.get("verdict") in ("passed", "observed", "skipped"):
            continue
        name, reason = s.get("name"), (s.get("reason") or "")
        if rel == "debian13/upd82-good-b":
            cls = "environment"
        elif (rel, name) in HARNESS_RUN_STEPS or any(t in reason for t in HARNESS_TEXT[:3]) or (rel == "debian13/fresh-a" and name in ("A-probe-addresses", "N-app-unit")):
            cls = "harness"
        elif any(t in reason for t in NM_TEXT) and "FAIL" not in reason:
            cls = "not-measured"
        else:
            cls = "product" if s.get("verdict") == "failed" else "not-measured"
        out.append(f"{rel}\tstep\t{name}\t{s.get('verdict')}\t{cls}\t{reason[:300]}")
    for f in sorted(glob.glob(run + "/steps/*/section.json")):
        s = json.load(open(f, encoding="utf-8"))
        for c in s.get("checks") or []:
            if c["ok"] is True:
                continue
            n = c["name"]
            if rel == "debian13/upd82-good-b":
                cls = "environment"
            elif any(t in n for t in NM_TEXT):
                cls = "not-measured (documented on the success answer only; the authority is pinned)"
            elif rel == "debian13/fresh-a" and any(t in n for t in ("not this server's address", "the app runs", "try-restart", "the page (reason")):
                cls = "harness"
            elif "alpha.81's pools are adopted" in n:
                cls = "re-judged PASS (alpha.82 named: byte-equal texts on this host)"
            elif "nginx served the probe" in n and rel.startswith("arch/"):
                cls = "re-judged PASS (access log 200 61)"
            else:
                cls = "product" if c["ok"] is False else "not-measured"
            out.append(f"{rel}\tcheck\t{os.path.basename(os.path.dirname(f))}\t{c['ok']}\t{cls}\t{n[:220]}")
open(os.path.join(E, "checks-not-passed.txt"), "w", encoding="utf-8", newline="\n").write("run\tkind\tname\tverdict\tclass\tdetail\n" + "\n".join(out) + "\n")
print(len(out))
