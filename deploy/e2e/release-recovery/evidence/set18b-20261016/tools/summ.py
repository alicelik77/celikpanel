"""set18b: the essentials of setup-summary.json. usage: summ.py DIR"""
import json, sys
from pathlib import Path

d = Path(sys.argv[1])
s = json.loads((d / "setup-summary.json").read_text(encoding="utf-8"))
P = lambda k, v: print(f"== {k}: {json.dumps(v, ensure_ascii=False)[:2500]}")
for k in ("c1_start_pressed_at", "c1_ready_variants", "c3_card_button", "c3_review_in_setup_pressed_at", "c3_d1", "c4_secondary_box", "c4_dns_before", "c4_dns_after",
          "c3_stop_pressed_at", "c3_stop_clicked", "c3_components_after_stop", "c2_purpose", "c2_components", "c2_mail_component", "c2_components_after",
          "c2_access_before_fill", "c2_access", "c2_review", "c2_review_variants", "c2_review_after_variants", "c2_start_pressed_at", "c2_ready_variants",
          "c2_components_after_run", "enroll_posts_total", "error", "c2_stopped", "ended_at"):
    if k in s:
        P(k, s[k])
P("c3_stopped", {k: s.get("c3_stopped", {}).get(k) for k in ("ready_screen", "at")})
for c in ("c1", "c2"):
    f = s.get(c) or {}
    P(c, {k: f.get(k) for k in ("started_at", "ready_at", "duration_s", "states", "queued_seen", "finishing_seen", "unknown_sentence_samples", "lab", "owner_actions",
                                "enrollment_presses", "enrollment_button_samples", "enroll_posts", "mail_dns_wait_seen", "mail_identity_seen", "failed", "not_finished")})
for k in ("c1_setup_read", "c3_setup_read", "c2_setup_read"):
    r = (s.get(k) or {}).get("result") or {}
    print(f"== {k}:", {w: (v.get("status"), (v.get("body") or {}).get("status"), (v.get("body") or {}).get("extension"), (v.get("body") or {}).get("superseded"), (v.get("body") or {}).get("id")) for w, v in r.items()} if isinstance(r, dict) else r)
