# case 3: same answers 124 vs 125 with time stamps dropped; lists the remaining differences by component. Lab only.
import json, sys
P = "/var/tmp/a125r1/perf"
DROP = {"package_version_read_at", "scanned_at", "scan_age_seconds", "checked_at", "read_at"}
def norm(x):
    if isinstance(x, dict): return {k: norm(v) for k, v in x.items() if k not in DROP}
    if isinstance(x, list): return [norm(v) for v in x]
    return x
for node in ("main", "backup"):
    for r in ("managed-services", "scan", "capabilities", "setup-components"):
        a = norm(json.load(open(f"{P}/a124-{node}-{r}.json"))); b = norm(json.load(open(f"{P}/a125-{node}-{r}.json")))
        if a == b: print(f"{node} {r}: identical"); continue
        def comps(d):
            L = d if isinstance(d, list) else (d.get("services") or d.get("items") or [])
            return {s.get("id"): s for s in L if isinstance(s, dict)}
        ca, cb = comps(a), comps(b)
        diffs = []
        for k in sorted(set(ca) | set(cb)):
            if ca.get(k) != cb.get(k):
                fa, fb = ca.get(k) or {}, cb.get(k) or {}
                keys = sorted(f for f in set(fa) | set(fb) if fa.get(f) != fb.get(f))
                diffs.append(f"{k}: " + ", ".join(f"{f}={json.dumps(fa.get(f))[:80]} -> {json.dumps(fb.get(f))[:80]}" for f in keys))
        top = sorted(k for k in set(a) | set(b) if isinstance(a, dict) and k not in ("services", "items") and a.get(k) != b.get(k)) if isinstance(a, dict) else []
        print(f"{node} {r}: differs in {len(diffs)} component(s){' and top-level ' + ','.join(top) if top else ''}")
        for d in diffs: print("   ", d[:400])
