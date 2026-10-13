"""set15: summarize a browser output's phases.json (reads only). usage: sum.py <ui-dir>"""
import json, sys
from pathlib import Path
d = Path(sys.argv[1]); p = json.load(open(d / "phases.json", encoding="utf-8"))
for ph in p["phases"]:
    name = ph["phase"]
    print("==", name, ph.get("started"), ph.get("seconds"), "scan_posts", ph.get("scan_posts"))
    if name == "domains":
        print("  first_state_row_shown", ph.get("first_state_row_shown"), "domains_reads", [(r["status"], r["body"]) for r in ph.get("domains_reads", [])])
        print("  sidebar stamp", ph["sidebar"].get("stamp"), "picture", ph["sidebar"].get("picture"))
    if name == "dashboard":
        print("  stale", ph.get("stale"))
        for x in ph.get("posts", []): print("  POST", x["url"], x["status"], x["start"], x["ms"], json.dumps(x.get("answer"))[:300])
        print("  conditional", ph.get("posts_conditional"), "rescan_link_seen", ph.get("rescan_link_seen"), "states", ph.get("states"))
        for s in ph.get("card_sequence", []): print("   ", s["at"][11:23], s["state"], s["lasted_ms"], (s["text"] or "")[:140])
        ar = ph.get("access_reads", [])
        print("  access reads", len(ar), [ (a["access"] or {}).get("valid_for_seconds") for a in ar][:5])
        print("  sidebar stamp", ph["sidebar"].get("stamp"))
        for a in ph.get("answers", [])[:4]: print("  answer", a["method"], a["url"], a["status"], json.dumps(a["answer"])[:250])
    if name == "components":
        print("  stale", ph.get("stale"))
        for part in ([("only", ph)] if "fresh" not in ph else [("fresh", ph["fresh"]), ("stale_load", ph["stale_load"])]):
            sub = part[1]
            for x in sub.get("posts", []): print("  POST", part[0], x["url"], x["status"], x["start"], x["ms"], json.dumps(x.get("answer"))[:300])
            print("  ", part[0], "posts", sub.get("scan_posts"), "conditional", sub.get("posts_conditional"), "alerts", sub.get("alerts"), "toasts", sub.get("toasts"))
    if name == "reads":
        for k, v in ph["reads"].items(): print(" ", k, v.get("status"), json.dumps(v.get("body"))[:400])
