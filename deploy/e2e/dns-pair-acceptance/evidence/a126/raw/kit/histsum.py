# histsum.py FILE [FROM TO]: point count, step, vcpus; with FROM/TO the samples inside the window. Lab only.
import json, sys
d = json.load(open(sys.argv[1]))
try: b = json.loads(d["body"])
except Exception: print("  history", d.get("status"), (d.get("body") or "")[:300]); sys.exit()
s = b.get("samples") or []
print("  range %s: status %s points %d (<=360: %s) step %ss sample %ss vcpus %s first %s last %s" % (b.get("range"), d.get("status"), len(s), len(s) <= 360, b.get("step_seconds"), b.get("sample_seconds"), b.get("vcpus"), s[0]["ts"] if s else None, s[-1]["ts"] if s else None))
if len(sys.argv) > 3 and sys.argv[2]:
    lo, hi = sys.argv[2], sys.argv[3]
    win = [p for p in s if lo <= p["ts"][:19] + "Z" <= hi or lo[:16] <= p["ts"][:16] <= hi[:16]]
    near = [p for p in s if p["ts"][:16] >= lo[:13]]
    for p in near[-16:]:
        mark = "*" if p in win else " "
        print("   %s %s cpu %5.1f cpu_peak %s load1 %.2f" % (mark, p["ts"], p["cpu"], p.get("cpu_peak"), p["load1"]))
    if win:
        inner = win[1:-1] or win
        print("  window points %d, cpu mean %.1f (inner %.1f), max %.1f; one busy core on %s vCPUs = %.0f %%" % (len(win), sum(p["cpu"] for p in win) / len(win), sum(p["cpu"] for p in inner) / len(inner), max(p["cpu"] for p in win), b.get("vcpus"), 100.0 / (b.get("vcpus") or 1)))
