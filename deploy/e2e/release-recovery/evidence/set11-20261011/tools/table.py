#!/usr/bin/env python3
"""set11: the cell x platform table from the staged evidence. Every verdict is the driver's, except where an expectation
the driver checked is one the record does not count (listed in DROPPED, with the reason); the verdict is then
recomputed from the remaining expectations and marked "re-judged". The raw path of each cell is its section.json,
whose `cells` entry holds every expectation with what was measured."""
import glob, json, os, sys
E = sys.argv[1]
RUNS = {"Debian 13": "debian13/run-c", "Ubuntu 24.04": "ubuntu/run-a", "Arch": "arch/run-a"}
EXTRA = {"Debian 13 (run-b)": "debian13/run-b"}
UPD = {"Debian 13": "debian13/update-startcheck"}
# An expectation of copies c-e that compares the read times of two independent watchers below a millisecond; from
# copy f on it is kept as detail only (set11_trial.py, probe_checks). Not counted in any run.
DROPPED = {"the file was there for every GET": "two independent watchers' read times, sub-millisecond: not a measurement"}


def verdict(expectations):
    held = [e["held"] for e in expectations]
    if any(h is False for h in held):
        return "FAIL"
    if any(h is None for h in held):
        return "NOT-MEASURED"
    return "PASS"


def cells_of(run):
    out = {}
    for f in sorted(glob.glob(os.path.join(E, run, "driver/steps/*/section.json"))):
        s = json.load(open(f, encoding="utf-8"))
        for name, c in (s.get("cells") or {}).items():
            kept = [e for e in c["expectations"] if not any(d in e["expectation"] for d in DROPPED)]
            v = verdict(kept)
            mark = " (re-judged)" if v != c["verdict"] else ""
            out[name] = (v + mark, os.path.relpath(f, E).replace(os.sep, "/"), [e["expectation"] for e in kept if e["held"] is not True],
                         c["verdict"])
    return out


data = {p: cells_of(r) for p, r in RUNS.items()}
for p, r in UPD.items():
    data[p].update(cells_of(r))
extra = {p: cells_of(r) for p, r in EXTRA.items()}
order = ["S0", "h", "i", "C", "A1", "A4", "A2", "A5", "A3", "A6", "B1", "B2", "B3", "B4", "B-before", "B-return"]
names = sorted({n for p in data for n in data[p]}, key=lambda n: order.index(n) if n in order else 99)
cols = list(data) + list(extra)
print("| Cell | " + " | ".join(cols) + " | raw (`<platform>/<run>/driver/` + the step's `section.json`) |")
print("| --- | " + " | ".join("---" for _ in cols) + " | --- |")
for n in names:
    row = []
    for p in data:
        v = data[p].get(n)
        row.append(v[0] if v else "not run")
    for p in extra:
        v = extra[p].get(n)
        row.append(v[0] if v else "not run")
    raw = (data["Debian 13"].get(n) or next((data[p][n] for p in data if n in data[p]), (None, "")))[1]
    print(f"| {n} | " + " | ".join(row) + f" | `{raw.split('/driver/')[-1] if raw else ''}` |")
print()
print("Driver verdicts recomputed without the dropped expectation:")
for p, d in list(data.items()) + list(extra.items()):
    for n, v in d.items():
        if "re-judged" in v[0]:
            print(f"- {p} {n}: driver {v[3]}, recomputed {v[0].split()[0]} (`{v[1]}`)")
print()
for p, d in list(data.items()) + list(extra.items()):
    for n in names:
        v = d.get(n)
        if v and not v[0].startswith("PASS"):
            print(f"- {p} {n} {v[0]}: " + "; ".join(v[2]) + f" (`{v[1]}`)")
