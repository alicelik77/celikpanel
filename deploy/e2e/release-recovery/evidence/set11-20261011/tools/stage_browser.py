#!/usr/bin/env python3
"""set11: copy one browser window's output (scratchpad/set11/browser/<hand>) into the evidence folder, without the
Chrome profile, and prune the screencast pictures of each load to: every picture up to 400 ms, the first picture at or
after each change of the painted kind, the picture nearest each stage time, and the last. The pruned names are listed
in PRUNED.txt (per load: received, kept by the script, kept after pruning). usage: stage_browser.py SRC DST"""
import json, os, shutil, sys

src, dst = sys.argv[1], sys.argv[2]
MARKS = [1000, 1600, 2000, 5000, 9000, 12000, 15200, 20000, 25200, 26500, 31000, 35000, 39000, 40500, 45000, 49000]
if os.path.exists(dst):
    sys.exit(f"refusing: {dst} exists")
shutil.copytree(src, dst, ignore=shutil.ignore_patterns(".chrome-profile-*"))
lines = ["Pictures pruned at staging (set11 stage_browser.py): kept every picture up to 400 ms after the navigation call, "
         "the first picture at or after each change of the painted kind, the picture nearest each of "
         + ", ".join(str(m) for m in MARKS) + " ms, and the last. `loads.jsonl` still lists every picture the script kept; "
         "the ones removed here are named below.", ""]
for root, _, files in os.walk(dst):
    if "loads.jsonl" not in files:
        continue
    for line in open(os.path.join(root, "loads.jsonl"), encoding="utf-8"):
        record = json.loads(line)
        shots = record.get("shots") or []
        if not shots:
            continue
        keep = {s["file"] for s in shots if s["ms"] <= 400}
        for run in record.get("painted") or []:
            after = [s for s in shots if s["ms"] >= run.get("from_goto_ms", 0)]
            if after:
                keep.add(after[0]["file"])
        for mark in MARKS:
            if mark <= record.get("watch_ms", 0) + 500:
                keep.add(min(shots, key=lambda s: abs(s["ms"] - mark))["file"])
        keep.add(shots[-1]["file"])
        removed = []
        for s in shots:
            path = os.path.join(root, s["file"])
            if s["file"] not in keep and os.path.exists(path):
                os.remove(path)
                removed.append(os.path.basename(s["file"]))
        lines.append(f"{os.path.relpath(root, dst)} {record['label']}: received {record.get('pictures_received')}, "
                     f"kept by the script {len(shots)}, kept after pruning {len(shots) - len(removed)}, removed {len(removed)}")
        if removed:
            lines.append("  removed: " + " ".join(sorted(removed)))
with open(os.path.join(dst, "PRUNED.txt"), "w", encoding="utf-8", newline="\n") as stream:
    stream.write("\n".join(lines) + "\n")
print("\n".join(l for l in lines if not l.startswith("  removed")))
