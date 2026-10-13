"""set14c: per platform, from browser/ui/{phases.json,cardlog.jsonl,requests.jsonl}: every scan POST per phase, the card
states in order with how long each lasted (to the next change in the same page), and the CPU deltas; written as
browser/summary.json next to them. Reads only those three files. usage: summarize.py <run-dir>"""
import json, sys
from pathlib import Path
run = Path(sys.argv[1]); ui = run / "browser" / "ui"
phases = json.load(open(ui / "phases.json", encoding="utf-8"))
cards = [json.loads(l) for l in open(ui / "cardlog.jsonl", encoding="utf-8")]
reqs = [json.loads(l) for l in open(ui / "requests.jsonl", encoding="utf-8")]
out = {"lang": phases["lang"], "viewport": phases["viewport"], "phases": []}
for p in phases["phases"]:
    name = p["phase"]
    rows = [c for c in cards if c.get("phase") == name or (name == "window" and False)]
    seq = []
    by_page = {}
    for c in cards:
        by_page.setdefault(c["page"], []).append(c)
    for page, items in by_page.items():
        states = [c for c in items if not c.get("ev")]
        for i, c in enumerate(states):
            if c.get("phase") != name: continue
            nxt = states[i + 1]["t"] if i + 1 < len(states) else None
            seq.append({"page": page, "at": c["at"], "state": c.get("state"), "vis": c.get("vis"),
                        "lasted_ms": (nxt - c["t"]) if nxt else None, "text": c.get("text")})
    seq.sort(key=lambda x: x["at"])
    posts = [{k: r.get(k) for k in ("page", "url", "status", "start", "end", "ms", "answer")} for r in reqs
             if r.get("phase") == name and r["method"] == "POST" and "managed-services/scan" in r["url"]]
    polls = sum(1 for r in reqs if r.get("phase") == name and "host-mutation-readiness" in r["url"])
    extra = {k: v for k, v in p.items() if k not in ("posts",)}
    out["phases"].append(extra | {"scan_posts_list": posts, "card_sequence": seq, "readiness_polls": polls})
(run / "browser" / "summary.json").write_text(json.dumps(out, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
for p in out["phases"]:
    print(p["phase"], "posts", [(x["url"].split("?")[-1][:22], x["status"], x["ms"], (x.get("answer") or {}).get("code")) for x in p["scan_posts_list"]])
    for s in p["card_sequence"]:
        print("   ", s["at"][11:23], s["page"], s["vis"], s["state"], s["lasted_ms"], (s["text"] or "")[:150])
