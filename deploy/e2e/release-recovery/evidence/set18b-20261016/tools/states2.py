"""set18b: one line per new state of a samples file (UTC, operation, titles, guidance start, enrollment buttons).
usage: states2.py FILE [GUIDANCE_CHARS]"""
import datetime, json, sys

n = int(sys.argv[2]) if len(sys.argv) > 2 else 200
for line in open(sys.argv[1], encoding="utf-8"):
    d = json.loads(line)
    if not (d.get("new_state") or d.get("marker")):
        continue
    t = datetime.datetime.fromtimestamp(d["t"] / 1000, datetime.UTC).strftime("%H:%M:%S.%f")[:-3] if d.get("t") else "?"
    o = d.get("operation") or {}
    steps = "".join(s["state"][0] for s in d.get("steps", []))
    print(t, o.get("phase"), o.get("status"), o.get("error"), "|", d.get("h1"), "|", d.get("progress_title"), "|", d.get("guidance_title"),
          "| fin:", d.get("finishing"), "| steps:", steps, "| enroll:", [(b["text"], b["disabled"]) for b in d.get("enroll_buttons") or []],
          "| checks:", [(c["id"], c["state"]) for c in d.get("checks", [])], "|", (d.get("guidance") or "")[:n], d.get("marker") or "")
