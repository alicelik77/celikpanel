#!/usr/bin/env python3
"""set11: one lab's decisive facts per section (read-only over the lab's driver records). usage: facts.py LAB_DIR"""
import glob, json, os, sys
base = glob.glob(os.path.join(sys.argv[1], "evidence/*/upd1/*/steps"))[-1]
print("steps:", base)


def section(pattern):
    found = sorted(glob.glob(os.path.join(base, pattern, "section.json")))
    return json.load(open(found[-1])) if found else None


def answer(a):
    b = a.get("body") or {}
    return {"status": a.get("status"), "code": b.get("code"), "reason": b.get("reason"), "detail": b.get("detail"),
            "vars": {k: v for k, v in (b.get("vars") or {}).items() if k != "detail"}}


for name, pattern in (("C", "*-c-directory-modes"), ("S0", "*-s0-sites")):
    s = section(pattern)
    if not s:
        continue
    for c in s["checks"]:
        if "include dir" in c["name"] or "include_dir" in c["name"] or "managed_dir" in c["name"] or "_base" in c["name"]:
            print(name, c["ok"], c["name"][:90], json.dumps(c["detail"])[:200])
for key, pattern in (("a1", "*-a1-*"), ("a4", "*-a4-*"), ("a2", "*-a2-*"), ("a5", "*-a5-*"), ("a3", "*-a3-*"), ("a6", "*-a6-*")):
    s = section(pattern)
    if not s:
        continue
    print("==", key, {n: c["verdict"] for n, c in (s.get("cells") or {}).items()}, "error:", (s.get("error") or "")[:200])
    d = s.get(key) or {}
    if key in ("a1", "a4", "a2"):
        print("  answer", answer(d.get("answer") or {}))
        print("  gets", [(g["host"], g["status"]) for g in (d.get("probe") or {}).get("gets", [])])
        for n, f in ((d.get("probe") or {}).get("probe_files") or {}).items():
            print("  probe file", n, f.get("final_mode"), f.get("owners")[:1] if f.get("owners") else None, f.get("events"))
    if key == "a5":
        print("  answer", json.dumps(d.get("answer"))[:600])
        print("  reload lines", d.get("nginx_reloaded_lines_during_the_read"), "managed dir events", len(d.get("managed_dir_events_during_the_read") or []))
        print("  rows", d.get("domains_row"))
    if key == "a3":
        print("  with block", answer(d["with_block"]["answer"]), [(g["host"], g["status"]) for g in d["with_block"]["probe"]["gets"]])
        print("  removed", answer(d["block_removed"]["answer"]), [(g["host"], g["status"]) for g in d["block_removed"]["probe"]["gets"]])
    if key == "a6":
        print("  ", json.dumps({k: v for k, v in d.items() if k in ("answer", "listening_80_443", "bound_to", "ledger_reason")})[:900])
        print("  gets", [(g["host"], g["status"]) for g in d.get("gets", [])])
    for c in s["checks"]:
        if c["ok"] is not True:
            print("  NOT", c["ok"], c["name"][:120])
for key, pattern in (("b1", "*-b1-*"), ("b2", "*-b2-*"), ("b3", "*-b3-*"), ("b4", "*-b4-*")):
    s = section(pattern)
    if not s:
        continue
    print("==", key, {n: c["verdict"] for n, c in (s.get("cells") or {}).items()}, "error:", (s.get("error") or "")[:200])
    d = s.get(key) or {}
    if d:
        print("  ", json.dumps({k: v for k, v in d.items() if k in ("page", "row", "ledger", "start_lines", "a_lines", "backups_after")})[:1500])
        if key == "b4":
            print("   take:", d["take"]["status"], (d["take"]["json"] or {}).get("code"), (d["take"]["json"] or {}).get("reason"))
    for c in s["checks"]:
        if c["ok"] is not True:
            print("  NOT", c["ok"], c["name"][:120], json.dumps(c["detail"])[:300])
