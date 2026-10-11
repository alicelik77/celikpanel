#!/usr/bin/env python3
"""set11: the eleventh native record: the corrections committed after the tenth (the certificate-validation probe
on a real nginx, the unwritable state of a locked file, the include directory mode, the session wait in a real Chrome,
the recovery service worker) and the headers on an automatic return, on disposable QEMU guests (lab.py).

Two cell kinds, built on set10's drivers and changing none of their steps.

``set11-debian13`` | ``set11-ubuntu`` | ``set11-arch`` (``Set11Trial`` over ``set10_trial.Set10Trial``): the code
installed FRESH (set6's steps: the names pinned first, preflight, origin, install, owner, licence fixture, setup,
set1's static site), then set10's S0 (two PHP sites A ``set11-a.test`` and B ``set11-b.test``) and h-i (a restart
with every site unchanged), then:

  C       the include directories as created: the owner's (``celikpanel-sites.d/<domain>``) and CelikPanel's
          (``celikpanel-managed.d/<domain>``), and their bases: mode, owner, group.
  A1      A's vhost owner-edited, CelikPanel's include lines kept: POST the certificate issuance. Watched: the probe
          file in the challenge directory (inotify), the loopback HTTP requests with their Host and nginx's answers
          (a packet reader on ``lo``), nginx's log files, the journal; the answer when the certificate authority (its
          name pinned to the guest's loopback) cannot be reached.
  A4      the same file, nginx stopped by the owner: POST issuance.
  A2      a kept file without the line that includes CelikPanel's directory: POST issuance.
  A5      after A2 the owner puts the line back and reloads nginx: GET site-config (it probes again while the ledger
          says certificate_validation), the Domains row and the dashboard before and after.
  A3      a kept file with an owner's validation-only block for ``mail.<domain>``: POST issuance with the mail name;
          then the owner removes that block: POST again.
  A6      the site's port-80 block bound to the guest's own address only (owner edit): POST issuance.
          (A1-A6 on Debian 13 and Ubuntu 24.04; on Arch only A2.)
  B1      A locked (``chattr +i``) while A, B and the static site need a render, the Panel started: the page's
          answer, the Domains row, and "check again" (a GET) watched for writes.
  B2      the owner unlocks A: "check again".
  B3      a General save of A.
  B4      "take" on an owner-edited A whose replacement fails (``chattr +i`` between the GET and the POST): the
          answer, the page and the Domains row.

With ``SET11_HAND`` set, the driver opens browser windows (set7's handover: ``ready.json``, a root-only password
file, ``restart-request-*``, ``done.json``) during B1 and B4 and after the sections, for a real Chrome on the host
(web/tools/browser-inspect/cold-load-set11.mjs): screenshots of the Domains list and the Configuration file page,
the session wait (cell D) and the recovery service worker (cell E).

``upd1-debian13-startcheck`` (``Set11UpdateTrial`` over ``set10_trial.Set10UpdateTrial``): set5's start-check cell
from the published alpha.81 with set10's sections: two sites created on alpha.81 before the update (plus R3 edited),
every vhost read after the automatic return.

The driver calls only the Panel's HTTP API; every native fact is read over SSH; owner actions and lab preparation on
the guest are recorded as such. No certificate authority and no licence service is contacted by the driver; the
certificate requests the Panel makes go to names pinned to the guest's loopback. Every result carries
``native_evidence: false``.

  set11_trial.py plan --cell set11-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set11_trial.py run  --cell set11-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import base64
import dataclasses
import json
import os
from pathlib import Path
import re
import secrets
import sys
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set10_trial as s10  # noqa: E402

s8, s6, s4, sw, base = s10.s8, s10.s6, s10.s4, s10.sw, s10.base
# set8's and set10's methods read their module names DOMAIN_A/DOMAIN_B at call time: the trial's constructor names
# the sites of this run set11 there (never at import, so importing this module changes no other module).
DOMAIN_A, DOMAIN_B, STATIC = "set11-a.test", "set11-b.test", s10.STATIC
HELPER11 = "guest_set11_native.py"
CELL_KIND, UPDATE_KIND = "set11-fresh-install", "set11-start-check-return"
CELLS = {
    "set11-debian13": sw.SettingsCell("set11-debian13", "debian13", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set11-ubuntu": sw.SettingsCell("set11-ubuntu", "ubuntu", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set11-arch": sw.SettingsCell("set11-arch", "arch", "web", sw.WEB_PRESET + ("postgresql",), False),
}
UPDATE_CELLS = ("upd1-debian13-startcheck",)
AVAILABLE = "/etc/nginx/sites-available"
INCLUDE_BASE = "/etc/nginx/celikpanel-sites.d"
MANAGED_BASE = "/etc/nginx/celikpanel-managed.d"
PROBE_NAME = re.compile(r"celikpanel-probe-[0-9a-f]{32}\Z")
PROBE_PATH = "/.well-known/acme-challenge/"
RUN_ROOT = Path(os.environ.get("SET11_RUN_ROOT", "/var/tmp/cp-set11-run"))
HAND_ROOT, SECRET_ROOT = RUN_ROOT / "hand", RUN_ROOT / "secret"
SETTLE = s10.SETTLE
SECTIONS_FULL = (
    ("S0-sites", "two PHP sites; header, digest, ledger row, include directory, nginx -T (set10's section)"),
    ("h-i-unchanged-start", "a restart with every site unchanged (set10's section; it records the Panel's text)"),
    ("C-directory-modes", "the include directories as created: mode, owner, group"),
    ("A1-probe-ready", "owner-edited file with CelikPanel's include lines: certificate issuance"),
    ("A4-nginx-stopped", "the same file with nginx stopped: certificate issuance"),
    ("A2-include-missing", "a kept file without the line including CelikPanel's directory: certificate issuance"),
    ("A5-include-back", "the owner puts the line back: the Configuration file read probes again"),
    ("A3-mail-name", "a validation-only block for mail.<domain>: present, then removed by the owner"),
    ("A6-bound-address", "the port-80 block bound to one address only: certificate issuance"),
    ("B1-locked-start", "A locked while A, B and the static site need a render; the page; check again"),
    ("B2-unlocked-recheck", "the owner unlocks A; check again"),
    ("B3-save-writes", "a General save of A after the unlock"),
    ("B4-take-refused", "take on an owner-edited A whose replacement fails"),
)
ARCH_SECTIONS = ("S0-sites", "h-i-unchanged-start", "C-directory-modes", "A2-include-missing", "B1-locked-start",
                 "B2-unlocked-recheck", "B3-save-writes", "B4-take-refused")


# -- pure rules (covered offline by test_set11_trial) ---------------------------------------------------------------

def probe_files(rows: list, challenge_dir: str | None) -> dict:
    """The probe files the inotify watcher saw in the challenge directory: per name, its events in order, the modes,
    owners and sizes seen, its bytes, and when it was created and removed (watcher clock)."""
    files: dict = {}
    for row in rows or []:
        name = row.get("name") or ""
        if not PROBE_NAME.fullmatch(name) or (challenge_dir and row.get("dir") != challenge_dir):
            continue
        item = files.setdefault(name, {"events": [], "modes": [], "owners": [], "sizes": [], "bytes": None,
                                       "created_t": None, "deleted_t": None})
        item["events"].extend(row.get("events") or [])
        if row.get("mode") is not None:
            item["modes"].append(row["mode"])
            item["owners"].append([row.get("uid"), row.get("gid")])
            item["sizes"].append(row.get("size"))
        if row.get("bytes"):
            item["bytes"] = row["bytes"]
        if "create" in (row.get("events") or []) and item["created_t"] is None:
            item["created_t"] = row.get("t")
        if "delete" in (row.get("events") or []):
            item["deleted_t"] = row.get("t")
    for item in files.values():
        item["final_mode"] = item["modes"][-1] if item["modes"] else None
    return files


def exchanges(rows: list, path_part: str = PROBE_PATH) -> list:
    """HTTP requests on loopback port 80 whose request line contains ``path_part``, each with the first answer sent
    back to its client port after it (watcher clock)."""
    found = []
    rows = sorted([r for r in rows or [] if isinstance(r.get("t"), (int, float))], key=lambda r: r["t"])
    for index, row in enumerate(rows):
        if not row.get("request") or path_part not in row["request"]:
            continue
        answer = next((r for r in rows[index + 1:] if r.get("answer") and r.get("to_port") == row.get("sport")), None)
        status = None
        if answer:
            parts = answer["answer"].split()
            status = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else None
        found.append({"t": row["t"], "host": row.get("host"), "request": row["request"], "src": row.get("src"),
                      "user_agent": row.get("user_agent"), "status": status,
                      "answer": answer.get("answer") if answer else None,
                      "location": answer.get("location") if answer else None,
                      "body_start": answer.get("body_start") if answer else None,
                      "answer_t": answer.get("t") if answer else None})
    return found


def syn_rows(rows: list, port: int, after: float | None = None) -> list:
    return [r for r in rows or [] if r.get("syn") and r.get("dport") == port and (after is None or r.get("t", 0) >= after)]


def answered_with(probe: dict, exchange: dict) -> bool:
    """nginx answered this request with exactly the probe file's bytes (status 200)."""
    body = probe.get("bytes")
    return exchange.get("status") == 200 and bool(body) and (exchange.get("body_start") or "") == body[:160]


def summary_of(probe_set: dict, gets: list) -> dict:
    return {"probe_files": {n: {k: v for k, v in f.items() if k != "bytes"} for n, f in probe_set.items()},
            "gets": [{k: g.get(k) for k in ("host", "status", "answer", "location", "request")} for g in gets]}


def verdict_of(checks: list) -> str:
    return s10.verdict_of(checks)


# -- the fresh-install cell -------------------------------------------------------------------------------------

def name_the_sites() -> None:
    s8.DOMAIN_A, s8.DOMAIN_B = DOMAIN_A, DOMAIN_B
    s10.DOMAIN_A, s10.DOMAIN_B = DOMAIN_A, DOMAIN_B


class Set11Trial(s10.Set10Trial):
    def __init__(self, *args: Any, **kwargs: Any) -> None:
        name_the_sites()
        super().__init__(*args, **kwargs)

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[HELPER11] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER11, HELPER11)[1]
        return helpers

    # -- guest calls ----------------------------------------------------------------------------------------------

    def native11(self, mode: str, **arguments: Any) -> dict:
        payload = base64.b64encode(json.dumps(arguments).encode()).decode()
        return self.helper(HELPER11, mode, "--args-b64", payload, timeout=420)

    def snap11(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.native11(mode, **arguments)
        self.native_sequence += 1
        name = f"native/{self.native_sequence:03d}-{s10.safe_name(label)}.json"
        self.record_json(name, value)
        self.current.setdefault("natives", []).append({"label": label, "mode": mode, "file": name,
                                                       "owner_action": mode.startswith("owner-")})
        value["_file"] = f"{self.step_dir}/{name}"
        return value

    def owner11(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.snap11("owner-" + label, mode, **arguments)
        self.current.setdefault("owner_actions", []).append({"label": label, "mode": mode, "at": value.get("at")})
        return value

    def a_id(self) -> int:
        return self.ids()[DOMAIN_A]

    def ledger_a(self, label: str) -> dict:
        reading = self.snap11(f"{label}-ledger", "read-ledger")
        return self.ledger_row(reading, self.a_id()) or {}

    def stat_a(self, label: str) -> dict:
        return self.snap11(f"{label}-stat", "read-stat", domain=DOMAIN_A)

    def acme(self, label: str, domains: list | None = None) -> dict:
        return self.snap11(f"{label}-acme", "read-acme", domains=domains or [DOMAIN_A])

    def dashboard(self, label: str) -> dict:
        entry = self.call(f"{label}: GET /api/v1/dashboard", "GET", "/api/v1/dashboard", timeout=120)
        body = entry["_parsed"] if isinstance(entry["_parsed"], dict) else {}
        return {"status": entry["status"], "expiring_certs": body.get("expiring_certs")}

    def ssl_state(self, label: str) -> dict:
        entry = self.call(f"{label}: GET ssl", "GET", f"/api/v1/domains/{self.a_id()}/ssl", timeout=120)
        return {"status": entry["status"], "body": entry["_parsed"]}

    def certificate_post(self, label: str, include_mail: bool = False) -> dict:
        body: dict = {"email": "hostmaster@" + DOMAIN_A, "auto_renew": False}
        if include_mail:
            body["include_mail"] = True
        started = time.time()
        entry = self.call(f"{label}: POST ssl/letsencrypt (the owner requests a certificate)", "POST",
                          f"/api/v1/domains/{self.a_id()}/ssl/letsencrypt", body, timeout=900)
        parsed = entry["_parsed"] if isinstance(entry["_parsed"], dict) else {}
        return {"status": entry["status"], "body": parsed, "seconds": round(time.time() - started, 2),
                "sent": body}

    def watched(self, label: str, action, extra_dirs: tuple = ()) -> dict:
        """Run ``action`` (an API call) with both watchers on: the challenge directory, CelikPanel's and the owner's
        include directories of A and sites-available; then nginx's log lines since and the journal window."""
        before = self.acme(f"{label}-before")
        site = before["sites"][DOMAIN_A]
        dirs = [d for d in (site.get("challenge_dir"), f"{MANAGED_BASE}/{DOMAIN_A}", f"{INCLUDE_BASE}/{DOMAIN_A}",
                            AVAILABLE) + tuple(extra_dirs) if d]
        offsets = self.snap11(f"{label}-log-offsets", "log-offsets")
        tag = re.sub(r"[^a-z0-9-]+", "-", label.lower()).strip("-")[:40] or "x"
        started = self.snap11(f"{label}-watch-start", "watch-start", tag=tag, dirs=dirs)
        result, error = None, None
        try:
            result = action()
        except Exception as exc:  # noqa: BLE001 - the watchers are stopped first; the error is raised after
            error = exc
        stopped = self.snap11(f"{label}-watch-stop", "watch-stop", tag=tag, settle=1.5)
        logs = self.snap11(f"{label}-nginx-logs", "read-logs", offsets=offsets["offsets"])
        window = self.window(label, float(before["epoch"]) - 1)
        if error is not None:
            raise error
        probe_set = probe_files(stopped["fs"], site.get("challenge_dir"))
        gets = exchanges(stopped["net"])
        return {"before": before, "dirs": dirs, "watch_started": started.get("active"), "fs": stopped["fs"],
                "net": stopped["net"], "probe_files": probe_set, "gets": gets, "logs": logs, "window": window,
                "result": result, "files": {"watch": stopped["_file"], "logs": logs["_file"], "journal": window["_file"]}}

    def journal_lines(self, window: dict, unit: str, needle: str | None = None) -> list:
        lines = window["by_unit"].get(unit) or []
        return [l for l in lines if needle is None or needle in l]

    # -- C --------------------------------------------------------------------------------------------------------

    def c_modes(self) -> None:
        reading = self.acme("c", [DOMAIN_A, DOMAIN_B, STATIC])
        checks = []
        for domain in (DOMAIN_A, DOMAIN_B, STATIC):
            site = reading["sites"][domain]
            for key in ("include_dir", "managed_dir"):
                d = site.get(key) or {}
                checks.append((f"{domain}: {key} is a directory, root-owned, mode 0755",
                               d.get("kind") == "directory" and d.get("uid") == 0 and d.get("mode_octal") == "0o755",
                               {k: d.get(k) for k in ("path", "kind", "mode_octal", "uid", "gid", "owner", "group")}))
        for key, d in reading["bases"].items():
            checks.append((f"{key} {d.get('path')} is a directory, root-owned, mode 0755",
                           d.get("kind") == "directory" and d.get("uid") == 0 and d.get("mode_octal") == "0o755",
                           {k: d.get(k) for k in ("path", "kind", "mode_octal", "uid", "gid", "owner", "group")}))
        self.current["acme"] = reading["_file"]
        self.judge("C", checks, {"nginx_processes": reading.get("nginx_processes")})

    # -- A ------------------------------------------------------------------------------------------------------

    def owner_edit_a(self, label: str, *edits: tuple) -> dict:
        """Lab: A back to the Panel's text; owner: an added location (CelikPanel's include lines kept) and the
        listed set11 edits, each followed by nginx -t and a reload."""
        self.restore10(label, DOMAIN_A)
        done = {"append": self.owner10(f"{label}-edit", "owner-vhost10", domain=DOMAIN_A, action="append-location",
                                       reload=True)}
        for action, extra in edits:
            done[action] = self.owner11(f"{label}-{action}", "owner-vhost11", domain=DOMAIN_A, action=action,
                                        reload=True, **extra)
        return done

    def probe_checks(self, obs: dict, names: list, served: list, ready_then_request: bool) -> list:
        """Expectations on the probe itself: one probe file created in the challenge directory with the documented
        name and mode 0644 and removed again; one loopback GET per name in order, each answered as expected."""
        files = obs["probe_files"]
        gets = [g for g in obs["gets"] if PROBE_PATH + "celikpanel-probe-" in g["request"]]
        probe = next(iter(files.values()), {})
        name = next(iter(files), None)
        checks = [
            ("one probe file, named celikpanel-probe-<32 hex>, was created in the challenge directory and removed",
             len(files) == 1 and "create" in probe.get("events", []) and "delete" in probe.get("events", []),
             {"challenge_dir": obs["before"]["sites"][DOMAIN_A].get("challenge_dir"),
              "files": {n: {k: f.get(k) for k in ("events", "modes", "owners", "sizes", "created_t", "deleted_t")}
                        for n, f in files.items()}}),
            ("its mode is 0644", probe.get("final_mode") == "0o644", {"modes": probe.get("modes"), "owners": probe.get("owners")}),
            ("the loopback GETs asked for that file under each validation name, in order",
             [g["host"] for g in gets] == names and all(name and name in g["request"] for g in gets),
             [{k: g.get(k) for k in ("host", "request", "src", "status", "answer")} for g in gets]),
            ("each name's answer as expected (200 with the probe's bytes = served)",
             len(gets) == len(names) and all(answered_with(probe, g) == want for g, want in zip(gets, served)),
             [{"host": g["host"], "status": g["status"], "served_the_probe": answered_with(probe, g),
               "location": g.get("location"), "body_start": (g.get("body_start") or "")[:80]} for g in gets]),
        ]
        # Not a check (set11 Debian, A1/A2): each watcher stamps an event when it reads it, and the two read
        # independently, so their sub-millisecond order says nothing (an answer was read 0.4 ms after the removal was
        # seen; a creation was seen 0.15 ms after the first request was read). The times are kept as detail; that
        # the file was there when nginx read it is shown by a 200 answer carrying the file's own bytes.
        checks[0][2]["watch_times"] = {"created_t": probe.get("created_t"), "deleted_t": probe.get("deleted_t"),
                                       "gets_t": [[g["t"], g.get("answer_t")] for g in gets]}
        last = max((g.get("answer_t") or g["t"]) for g in gets) if gets else None
        to_ca = syn_rows(obs["net"], 443, last)
        if ready_then_request:
            checks.append(("after the probe, a connection to port 443 on the loopback (certbot asking the pinned "
                           "certificate authority name) was opened", bool(to_ca),
                           {"syn_443_after_probe": to_ca[:6]}))
        else:
            checks.append(("nothing was requested: no connection to port 443 after the probe", not to_ca,
                           {"syn_443_after_probe": to_ca[:6]}))
        return checks

    def file_untouched(self, before: dict, after: dict) -> tuple:
        b, a = before["vhost"], after["vhost"]
        return ("A's vhost is untouched (sha256, inode, mtime)",
                (b.get("sha256"), b.get("inode"), b.get("mtime_ns")) == (a.get("sha256"), a.get("inode"), a.get("mtime_ns")),
                {k: [b.get(k), a.get(k)] for k in ("sha256", "inode", "mtime_ns")})

    def a1(self) -> None:
        self.owner_edit_a("a1")
        names = [DOMAIN_A, "www." + DOMAIN_A]
        before = self.stat_a("a1-0")
        ledger0 = self.ledger_a("a1-0")
        obs = self.watched("a1", lambda: self.certificate_post("a1"))
        after = self.stat_a("a1-1")
        ledger1 = self.ledger_a("a1-1")
        ssl = self.ssl_state("a1 after")
        answer = obs["result"]
        body = answer["body"]
        agent = self.journal_lines(obs["window"], "celikpanel-agent.service")
        panel = self.journal_lines(obs["window"], "celikpanel-panel.service")
        checks = self.probe_checks(obs, names, [True, True], ready_then_request=True)
        checks += [
            ("the answer is the certificate authority's failure, not a refusal by the file "
             "(502 CERTIFICATE_ISSUE_FAILED)", answer["status"] == 502 and body.get("code") == "CERTIFICATE_ISSUE_FAILED",
             {k: body.get(k) for k in ("code", "reason", "error", "vars")} | {"status": answer["status"], "seconds": answer["seconds"]}),
            ("its reason is authority_unreachable (the documented kind for an authority this server cannot reach)",
             body.get("reason") == "authority_unreachable", body.get("reason")),
            self.file_untouched(before, after),
            ("no ledger reason was written (state_reason empty before and after)",
             not ledger0.get("state_reason") and not ledger1.get("state_reason"),
             {"before": {k: ledger0.get(k) for k in ("state", "state_reason")}, "after": {k: ledger1.get(k) for k in ("state", "state_reason")}}),
        ]
        self.current["a1"] = {"answer": answer, "probe": summary_of(obs["probe_files"], obs["gets"]),
                              "agent_lines": [l for l in agent if "certbot" in l.lower() or "probe" in l.lower()
                                              or "certificate" in l.lower()][:40],
                              "panel_lines": [l for l in panel if "SSL" in l or "certificate" in l.lower()][:40],
                              "ssl_after": ssl, "files": obs["files"], "nginx_logs": obs["logs"].get("logs")}
        self.judge("A1", checks, self.current["a1"])

    def a4(self) -> None:
        # The file stays as A1 left it (owner-edited, include lines kept).
        before = self.stat_a("a4-0")
        ledger0 = self.ledger_a("a4-0")
        stopped = self.owner11("a4-nginx-stop", "owner-nginx", action="stop")
        try:
            obs = self.watched("a4", lambda: self.certificate_post("a4"))
        finally:
            started = self.owner11("a4-nginx-start", "owner-nginx", action="start")
        after = self.stat_a("a4-1")
        ledger1 = self.ledger_a("a4-1")
        answer, body = obs["result"], obs["result"]["body"]
        files = obs["probe_files"]
        probe = next(iter(files.values()), {})
        syn80 = syn_rows(obs["net"], 80)
        rst80 = [r for r in obs["net"] if r.get("rst_from") == 80]
        checks = [
            ("nginx was stopped by the owner (inactive, nothing listening on port 80)",
             stopped.get("is_active") in ("inactive", "failed") and not stopped.get("listening_80"),
             {k: stopped.get(k) for k in ("is_active", "listening_80")}),
            ("503 CERTIFICATE_VALIDATION_UNKNOWN with detail unknown",
             answer["status"] == 503 and body.get("code") == "CERTIFICATE_VALIDATION_UNKNOWN" and body.get("detail") == "unknown",
             {k: body.get(k) for k in ("code", "detail", "error")} | {"status": answer["status"]}),
            ("the probe file was written and removed", len(files) == 1 and "delete" in probe.get("events", []),
             {n: {k: f.get(k) for k in ("events", "modes")} for n, f in files.items()}),
            ("the probe's connection to port 80 was refused (SYN answered by RST; no HTTP answer)",
             bool(syn80) and bool(rst80) and not [g for g in obs["gets"] if g.get("status")],
             {"syn_80": syn80[:4], "rst_from_80": rst80[:4]}),
            ("no connection to port 443 (nothing was requested)", not syn_rows(obs["net"], 443), syn_rows(obs["net"], 443)[:4]),
            ("no ledger reason was written", (ledger1.get("state_reason") or "") == (ledger0.get("state_reason") or ""),
             {"before": ledger0.get("state_reason"), "after": ledger1.get("state_reason")}),
            self.file_untouched(before, after),
            ("nginx was started again by the owner", started.get("is_active") == "active", started.get("is_active"))]
        self.current["a4"] = {"answer": answer, "probe": summary_of(files, obs["gets"]), "files": obs["files"]}
        self.judge("A4", checks, self.current["a4"])

    def a2(self) -> None:
        edits = self.owner_edit_a("a2", ("remove-managed-include", {}))
        names = [DOMAIN_A, "www." + DOMAIN_A]
        before = self.stat_a("a2-0")
        rows0 = self.domains_rows("a2 before")
        obs = self.watched("a2", lambda: self.certificate_post("a2"))
        after = self.stat_a("a2-1")
        ledger1 = self.ledger_a("a2-1")
        rows1 = self.domains_rows("a2 after")
        dash = self.dashboard("a2 after")
        answer, body = obs["result"], obs["result"]["body"]
        gets = [g for g in obs["gets"] if PROBE_PATH + "celikpanel-probe-" in g["request"]]
        first = gets[0] if gets else {}
        vars_ = body.get("vars") or {}
        checks = self.probe_checks(obs, names, [False, False], ready_then_request=False)
        checks += [
            ("409 SITE_CONFIG_OWNER_EDITED, reason certificate_validation, detail include_missing",
             answer["status"] == 409 and body.get("code") == "SITE_CONFIG_OWNER_EDITED"
             and body.get("reason") == "certificate_validation" and body.get("detail") == "include_missing",
             {k: body.get(k) for k in ("code", "reason", "detail", "vars", "error")} | {"status": answer["status"]}),
            ("vars.name is the first site name and vars.status nginx's status for it (as the packet reader saw it)",
             vars_.get("name") == DOMAIN_A and str(vars_.get("status")) == str(first.get("status")),
             {"vars": vars_, "first_get": {k: first.get(k) for k in ("host", "status", "answer")}}),
            ("vars.include is the line to add", vars_.get("include") == f"include {MANAGED_BASE}/{DOMAIN_A}/*.conf;",
             vars_.get("include")),
            ("the ledger reason is certificate_validation", ledger1.get("state_reason") == "certificate_validation",
             {k: ledger1.get(k) for k in ("state", "state_reason")}),
            self.file_untouched(before, after)]
        self.s10["a2_rows_after"] = rows1.get(DOMAIN_A)
        self.s10["a2_dashboard"] = dash
        self.current["a2"] = {"answer": answer, "probe": summary_of(obs["probe_files"], obs["gets"]),
                              "removed_lines": edits["remove-managed-include"].get("changed"),
                              "domains_row": {"before": rows0.get(DOMAIN_A), "after": rows1.get(DOMAIN_A)},
                              "dashboard_after": dash, "files": obs["files"], "nginx_logs": obs["logs"].get("logs")}
        self.judge("A2", checks, self.current["a2"])

    def a5(self) -> None:
        ledger0 = self.ledger_a("a5-0")
        rows0 = self.domains_rows("a5 before")
        dash0 = self.dashboard("a5 before")
        back = self.owner11("a5-add-managed-include", "owner-vhost11", domain=DOMAIN_A, action="add-managed-include", reload=True)
        acme0 = self.acme("a5-0-challenge-file")
        before = self.stat_a("a5-0")
        obs = self.watched("a5", lambda: self.site_config("a5 GET site-config (the owner reloads the page)", self.a_id()))
        after = self.stat_a("a5-1")
        ledger1 = self.ledger_a("a5-1")
        rows1 = self.domains_rows("a5 after")
        dash1 = self.dashboard("a5 after")
        again = self.site_config("a5 GET site-config again (no reason: no probe expected)", self.a_id())
        shown = obs["result"]["body"]
        names = [DOMAIN_A, "www." + DOMAIN_A]
        nginx_reloads = self.journal_lines(obs["window"], "nginx.service", "Reloaded")
        managed_events = [r for r in obs["fs"] if r.get("dir") == f"{MANAGED_BASE}/{DOMAIN_A}"]
        a_row0, a_row1 = rows0.get(DOMAIN_A) or {}, rows1.get(DOMAIN_A) or {}
        mine = lambda d: [e for e in (d.get("expiring_certs") or []) if isinstance(e, dict) and e.get("domain_name") == DOMAIN_A]
        checks = [
            ("the reason was certificate_validation before the read", ledger0.get("state_reason") == "certificate_validation",
             ledger0.get("state_reason")),
            ("the owner's line is back and nginx -t passed, nginx reloaded",
             (back.get("nginx_test") or {}).get("returncode") == 0 and (back.get("reload") or {}).get("returncode") == 0,
             {"nginx_test": (back.get("nginx_test") or {}).get("returncode"), "reload": (back.get("reload") or {}).get("returncode")}),
        ] + self.probe_checks(obs, names, [True, True], ready_then_request=False)[:5] + [
            ("GET site-config answers 200 with validation ready and resolved_reason certificate_validation",
             obs["result"]["status"] == 200 and shown.get("validation") == "ready"
             and shown.get("resolved_reason") == "certificate_validation",
             {k: shown.get(k) for k in ("state", "validation", "resolved_reason", "pending_reason", "challenge_file",
                                        "validation_name", "validation_status", "managed_include")}),
            ("the ledger reason is cleared", not ledger1.get("state_reason"), {k: ledger1.get(k) for k in ("state", "state_reason")}),
            ("the Domains row carried the reason before and not after",
             a_row0.get("pending_reason") == "certificate_validation" and not a_row1.get("pending_reason"),
             {"before": a_row0, "after": a_row1}),
            ("the dashboard entry for the site clears", None if not mine(dash0) else not mine(dash1),
             {"before": mine(dash0), "after": mine(dash1),
              "why_not_measured": None if mine(dash0) else "no dashboard entry existed before: entries are certificates "
                                                           "waiting for the owner, and this site has no certificate"}),
            ("A's vhost was not written by the read", (before["vhost"].get("sha256"), before["vhost"].get("inode")) ==
             (after["vhost"].get("sha256"), after["vhost"].get("inode")),
             {k: [before["vhost"].get(k), after["vhost"].get(k)] for k in ("sha256", "inode")}),
            ("a second read without the reason asks for no probe (no probe_validation)", again["body"].get("validation") in (None, ""),
             {k: again["body"].get(k) for k in ("state", "validation", "resolved_reason", "pending_reason")})]
        challenge = (acme0["sites"][DOMAIN_A].get("managed_entries") or [])
        self.current["a5"] = {"answer": {"status": obs["result"]["status"], "body": {k: shown.get(k) for k in (
            "state", "validation", "resolved_reason", "pending_reason", "challenge_file", "actions", "managed_dir",
            "managed_include", "validation_name", "validation_status")}},
            "challenge_file_before_the_read": [{k: c.get(k) for k in ("path", "sha256", "inode", "mtime_ns", "header")} for c in challenge],
            "challenge_file_state_in_the_answer": shown.get("challenge_file"),
            "nginx_reloaded_lines_during_the_read": nginx_reloads,
            "managed_dir_events_during_the_read": managed_events,
            "domains_row": {"before": a_row0, "after": a_row1}, "dashboard": {"before": dash0, "after": dash1},
            "probe": summary_of(obs["probe_files"], obs["gets"]), "files": obs["files"]}
        self.judge("A5", checks, self.current["a5"])

    def a3(self) -> None:
        self.owner_edit_a("a3", ("add-mail-block", {}))
        names = [DOMAIN_A, "www." + DOMAIN_A, "mail." + DOMAIN_A]
        before = self.stat_a("a3-0")
        obs = self.watched("a3-with-block", lambda: self.certificate_post("a3 with the owner's mail block", include_mail=True))
        answer, body = obs["result"], obs["result"]["body"]
        checks = [(f"with the block: {t}", ok, d) for t, ok, d in self.probe_checks(obs, names, [True, True, True], True)]
        checks.append(("with the block: the answer is the certificate authority's failure (502 CERTIFICATE_ISSUE_FAILED)",
                       answer["status"] == 502 and body.get("code") == "CERTIFICATE_ISSUE_FAILED",
                       {k: body.get(k) for k in ("code", "reason", "error")} | {"status": answer["status"]}))
        removed = self.owner11("a3-remove-mail-block", "owner-vhost11", domain=DOMAIN_A, action="remove-mail-block", reload=True)
        mid = self.stat_a("a3-1")
        obs2 = self.watched("a3-block-removed", lambda: self.certificate_post("a3 after the owner removed the mail block",
                                                                              include_mail=True))
        after = self.stat_a("a3-2")
        ledger = self.ledger_a("a3-2")
        answer2, body2 = obs2["result"], obs2["result"]["body"]
        gets2 = [g for g in obs2["gets"] if PROBE_PATH + "celikpanel-probe-" in g["request"]]
        mail_get = next((g for g in gets2 if g["host"] == "mail." + DOMAIN_A), {})
        vars_ = body2.get("vars") or {}
        checks += [(f"block removed: {t}", ok, d) for t, ok, d in self.probe_checks(obs2, names, [True, True, False], False)]
        checks += [
            ("block removed: 409 SITE_CONFIG_OWNER_EDITED, reason certificate_validation, detail names_missing",
             answer2["status"] == 409 and body2.get("code") == "SITE_CONFIG_OWNER_EDITED"
             and body2.get("reason") == "certificate_validation" and body2.get("detail") == "names_missing",
             {k: body2.get(k) for k in ("code", "reason", "detail", "vars", "error")} | {"status": answer2["status"]}),
            ("block removed: vars.name is mail.<domain> and vars.status nginx's status for it",
             vars_.get("name") == "mail." + DOMAIN_A and str(vars_.get("status")) == str(mail_get.get("status")),
             {"vars": vars_, "mail_get": {k: mail_get.get(k) for k in ("host", "status", "answer", "body_start")}}),
            ("block removed: the ledger reason is certificate_validation", ledger.get("state_reason") == "certificate_validation",
             ledger.get("state_reason")),
            ("the file was not written by either request (only the owner's edit changed it)",
             before["vhost"].get("inode") == mid["vhost"].get("inode") == after["vhost"].get("inode")
             and mid["vhost"].get("sha256") == after["vhost"].get("sha256"),
             {"inode": [before["vhost"].get("inode"), mid["vhost"].get("inode"), after["vhost"].get("inode")]})]
        self.current["a3"] = {"with_block": {"answer": answer, "probe": summary_of(obs["probe_files"], obs["gets"]), "files": obs["files"]},
                              "block_removed": {"answer": answer2, "probe": summary_of(obs2["probe_files"], obs2["gets"]),
                                                "files": obs2["files"], "removed_lines": removed.get("changed"),
                                                "nginx_logs": obs2["logs"].get("logs")}}
        self.judge("A3", checks, self.current["a3"])

    def a6(self) -> None:
        address = (self.snap11("a6-addresses", "read-addresses").get("addresses") or [None])[0]
        if not address:
            self.judge("A6", [("the guest has a global IPv4 address to bind to", None, None)])
            return
        edits = self.owner_edit_a("a6", ("bind-address", {"address": address}))
        before = self.stat_a("a6-0")
        acme = self.acme("a6-listening")
        obs = self.watched("a6", lambda: self.certificate_post("a6"))
        after = self.stat_a("a6-1")
        ledger = self.ledger_a("a6-1")
        answer, body = obs["result"], obs["result"]["body"]
        gets = [g for g in obs["gets"] if PROBE_PATH + "celikpanel-probe-" in g["request"]]
        measured = {"answer": {k: body.get(k) for k in ("code", "reason", "detail", "vars", "error")} | {"status": answer["status"]},
                    "gets": [{k: g.get(k) for k in ("host", "status", "answer", "body_start")} for g in gets],
                    "listening_80_443": acme.get("listening_80_443"), "bound_to": address,
                    "changed": edits["bind-address"].get("changed"), "ledger_reason": ledger.get("state_reason")}
        unknown = answer["status"] == 503 and body.get("code") == "CERTIFICATE_VALIDATION_UNKNOWN"
        missing = answer["status"] == 409 and body.get("detail") == "include_missing"
        checks = [
            ("the owner's block is bound to the guest's address only and nginx -t passed",
             (edits["bind-address"].get("nginx_test") or {}).get("returncode") == 0, edits["bind-address"].get("changed")),
            ("the loopback probe asked each site name", [g["host"] for g in gets] == [DOMAIN_A, "www." + DOMAIN_A],
             [g["host"] for g in gets]),
            ("the answer is one the documentation names for this case (unknown with nothing on the loopback's port 80, "
             "or include_missing when another block listens on the wildcard address)", unknown or missing, measured),
            self.file_untouched(before, after)]
        self.current["a6"] = measured | {"files": obs["files"], "nginx_logs": obs["logs"].get("logs")}
        self.judge("A6", checks, self.current["a6"])
        # Lab: A back to the Panel's text; a read ends a reason the request wrote (the probe is served again).
        self.restore10("a6-end", DOMAIN_A)
        self.current["a6_cleanup_read"] = self.site_config("a6 cleanup: GET site-config after the lab restore", self.a_id())["body"].get("resolved_reason")

    # -- B --------------------------------------------------------------------------------------------------------

    def b1(self) -> None:
        refs = self.s10["ref"]
        self.restore10("b1", DOMAIN_A)
        self.panel10("b1", "stop")
        self.snap10("b1-lab-stale", "lab-stale-managed", domains=[DOMAIN_A, DOMAIN_B, STATIC])
        self.owner11("b1-chattr-plus-i", "owner-chattr", domain=DOMAIN_A, flag="+i")
        locked = self.read10("b1-1-locked")
        since = self.guest_epoch()
        fired = self.panel10("b1", "start")
        time.sleep(SETTLE)
        after = self.read10("b1-2-after-the-start")
        window = self.window("b1", since - 1)
        shown = self.site_config("b1 after the start", self.a_id())
        rows = self.domains_rows("b1 after the start")
        ledger = self.ledger_row(after, self.a_id()) or {}
        body = shown["body"]
        a, l = after["sites"][DOMAIN_A], locked["sites"][DOMAIN_A]
        written = {d: {"inode": [locked["sites"][d]["vhost"].get("inode"), after["sites"][d]["vhost"].get("inode")],
                       "panel_text": after["sites"][d]["vhost"].get("sha256") == refs[d]["sha256"]} for d in (DOMAIN_B, STATIC)}
        # "Check again": the page's GET, watched for any write to A's files and for a reload.
        stat0 = self.stat_a("b1-recheck-0")
        recheck_since = float(stat0["epoch"])
        browser = {}

        def recheck() -> dict:
            browser["window"] = self.browser_window("b-unwritable", 1500, {"domains": [{"name": DOMAIN_A, "id": self.a_id()}],
                                                                             "click": "recheck"})
            return self.site_config("b1 check again (GET site-config)", self.a_id())
        obs = self.watched("b1-recheck", recheck)
        stat1 = self.stat_a("b1-recheck-1")
        again = obs["result"]["body"]
        a_events = [r for r in obs["fs"] if r.get("dir") == AVAILABLE and (r.get("name") or "").startswith(DOMAIN_A)]
        writes = [l for l in self.journal_lines(obs["window"], "celikpanel-panel.service", f"site configuration {DOMAIN_A} ")
                  if s10.journal_epoch(l) and s10.journal_epoch(l) >= recheck_since]
        reloads = [l for l in self.journal_lines(obs["window"], "nginx.service", "Reloaded")
                   if s10.journal_epoch(l) and s10.journal_epoch(l) >= recheck_since]
        page_keys = ("state", "reason", "system_error", "protection", "detail", "actions", "file_sha256", "render_sha256")
        checks = [
            ("A carried the immutable flag before the start", bool(l["vhost"].get("immutable")), l["vhost"].get("lsattr")),
            ("the Panel started", bool(fired.get("ok")), {k: fired.get(k) for k in ("is_active", "ok")}),
            ("A untouched and its link intact; B and the static site written",
             a["vhost"].get("sha256") == l["vhost"].get("sha256") and a["vhost"].get("inode") == l["vhost"].get("inode")
             and a["enabled"].get("kind") == "symlink" and all(v["inode"][0] != v["inode"][1] and v["panel_text"] for v in written.values()),
             {"a": self.brief(a)["vhost"], "others": written}),
            ("the ledger stores unreadable + write_refused", ledger.get("state") == "unreadable" and ledger.get("state_reason") == "write_refused",
             {k: ledger.get(k) for k in ("state", "state_reason", "file_sha256")}),
            ("GET site-config: unwritable, write_refused, EPERM, immutable, a difference, actions [recheck]",
             body.get("state") == "unwritable" and body.get("reason") == "write_refused" and body.get("system_error") == "EPERM"
             and body.get("protection") == "immutable" and bool(body.get("diff")) and body.get("actions") == ["recheck"],
             {k: body.get(k) for k in page_keys} | {"diff_lines": len((body.get("diff") or "").splitlines())}),
            ("the Domains row says unwritable", (rows.get(DOMAIN_A) or {}).get("state") == "unwritable", rows.get(DOMAIN_A)),
            ("check again answers the same state", {k: again.get(k) for k in page_keys} == {k: body.get(k) for k in page_keys},
             {k: again.get(k) for k in page_keys}),
            ("check again wrote nothing: A's sha256, inode and mtime unchanged, no inotify event on A's files",
             (stat0["vhost"].get("sha256"), stat0["vhost"].get("inode"), stat0["vhost"].get("mtime_ns"))
             == (stat1["vhost"].get("sha256"), stat1["vhost"].get("inode"), stat1["vhost"].get("mtime_ns")) and not a_events,
             {"before": {k: stat0["vhost"].get(k) for k in ("sha256", "inode", "mtime_ns")},
              "after": {k: stat1["vhost"].get(k) for k in ("sha256", "inode", "mtime_ns")}, "events_on_a": a_events[:10]}),
            ("check again logged no site-configuration write line and nginx was not reloaded",
             not [w for w in writes if "written" in w] and not reloads, {"site_lines": writes, "reloads": reloads})]
        self.current["b1"] = {"start_lines": s10.start_lines(window["by_unit"].get("celikpanel-panel.service") or []),
                              "a_lines": s10.site_lines(window["by_unit"].get("celikpanel-panel.service") or [], DOMAIN_A),
                              "page": {k: body.get(k) for k in page_keys}, "row": rows.get(DOMAIN_A),
                              "browser": browser.get("window"), "files": obs["files"], "journal": window["_file"]}
        if body.get("diff"):
            self.keep_text("files/b1-site-config-diff.txt", body["diff"])
        self.judge("B1", checks, self.current["b1"])

    def b2(self) -> None:
        unlocked = self.owner11("b2-chattr-minus-i", "owner-chattr", domain=DOMAIN_A, flag="-i")
        stat0 = self.stat_a("b2-0")
        obs = self.watched("b2-recheck", lambda: self.site_config("b2 check again after the owner unlocked A", self.a_id()))
        stat1 = self.stat_a("b2-1")
        rows = self.domains_rows("b2 after")
        body = obs["result"]["body"]
        page_keys = ("state", "reason", "system_error", "protection", "detail", "actions")
        a_events = [r for r in obs["fs"] if r.get("dir") == AVAILABLE and (r.get("name") or "").startswith(DOMAIN_A)]
        checks = [
            ("the owner cleared the flag", not (unlocked.get("after") or {}).get("immutable"), (unlocked.get("after") or {}).get("lsattr")),
            ("check again: state unwritable, protection none (no protection found now; written at the next start or save)",
             body.get("state") == "unwritable" and body.get("protection") == "none", {k: body.get(k) for k in page_keys}),
            ("the Domains row still says unwritable", (rows.get(DOMAIN_A) or {}).get("state") == "unwritable", rows.get(DOMAIN_A)),
            ("check again wrote nothing (sha256, inode, mtime; no inotify event on A's files)",
             (stat0["vhost"].get("sha256"), stat0["vhost"].get("inode"), stat0["vhost"].get("mtime_ns"))
             == (stat1["vhost"].get("sha256"), stat1["vhost"].get("inode"), stat1["vhost"].get("mtime_ns")) and not a_events,
             {"events_on_a": a_events[:10]})]
        self.current["b2"] = {"page": {k: body.get(k) for k in page_keys}, "row": rows.get(DOMAIN_A), "files": obs["files"]}
        self.judge("B2", checks, self.current["b2"])

    def b3(self) -> None:
        before = self.read10("b3-0", domains=[DOMAIN_A])
        since = self.guest_epoch()
        saved = self.general_save10("b3 save", self.a_id())
        time.sleep(SETTLE)
        after = self.read10("b3-1", domains=[DOMAIN_A])
        window = self.window("b3", since - 1)
        shown = self.site_config("b3 after the save", self.a_id())
        rows = self.domains_rows("b3 after the save")
        b, a = before["sites"][DOMAIN_A]["vhost"], after["sites"][DOMAIN_A]["vhost"]
        ledger = self.ledger_row(after, self.a_id()) or {}
        checks = [
            ("the save answers 200", saved["status"] == 200, saved),
            ("A was written: new inode, CelikPanel's text with a valid header",
             a.get("inode") != b.get("inode") and a.get("sha256") == self.s10["ref"][DOMAIN_A]["sha256"]
             and bool((a.get("header") or {}).get("valid")), {k: [b.get(k), a.get(k)] for k in ("inode", "sha256")}),
            ("the page and the Domains row say managed_unchanged; the ledger too",
             shown["body"].get("state") == "managed_unchanged" and (rows.get(DOMAIN_A) or {}).get("state") in ("managed_unchanged", None)
             and ledger.get("state") == "managed_unchanged",
             {"page": shown["body"].get("state"), "row": rows.get(DOMAIN_A), "ledger": {k: ledger.get(k) for k in ("state", "state_reason")}}),
            ("a journal line names the write", any("written" in l for l in s10.site_lines(window["by_unit"].get("celikpanel-panel.service") or [], DOMAIN_A)),
             s10.site_lines(window["by_unit"].get("celikpanel-panel.service") or [], DOMAIN_A))]
        self.judge("B3", checks, {"journal": window["_file"]})

    def b4(self) -> None:
        self.restore10("b4", DOMAIN_A)
        self.owner10("b4-edit", "owner-vhost10", domain=DOMAIN_A, action="append-location", reload=True)
        shown = self.site_config("b4 shown (owner_edited)", self.a_id())
        lock = self.owner11("b4-chattr-plus-i", "owner-chattr", domain=DOMAIN_A, flag="+i")
        before = self.stat_a("b4-0")
        since = float(before["epoch"])
        body = {"file_sha256": shown["body"].get("file_sha256"), "render_sha256": shown["body"].get("render_sha256")}
        taken = self.post_raw("b4 take while the file is locked", f"/api/v1/domains/{self.a_id()}/site-config/take", body,
                              secrets.token_hex(16))
        time.sleep(SETTLE)
        after = self.stat_a("b4-1")
        window = self.window("b4", since - 1)
        page = self.site_config("b4 the page after the refused take", self.a_id())
        rows = self.domains_rows("b4 after the refused take")
        ledger = self.ledger_a("b4-1")
        browser = self.browser_window("b-take-refused", 1500, {"domains": [{"name": DOMAIN_A, "id": self.a_id()}], "click": None})
        page_keys = ("state", "reason", "system_error", "protection", "detail", "actions")
        unlocked = self.owner11("b4-chattr-minus-i", "owner-chattr", domain=DOMAIN_A, flag="-i")
        checks = [
            ("the owner-edited file was shown as owner_edited with the three choices",
             shown["body"].get("state") == "owner_edited" and shown["body"].get("actions") == ["keep", "take", "merge"],
             {k: shown["body"].get(k) for k in ("state", "actions")}),
            ("take answers 409 SITE_CONFIG_UNWRITABLE, reason write_refused",
             taken["status"] == 409 and taken["body"].get("code") == "SITE_CONFIG_UNWRITABLE" and taken["body"].get("reason") == "write_refused",
             taken["entry"]),
            ("the owner's file is untouched (sha256, inode)",
             (before["vhost"].get("sha256"), before["vhost"].get("inode")) == (after["vhost"].get("sha256"), after["vhost"].get("inode")),
             {k: [before["vhost"].get(k), after["vhost"].get(k)] for k in ("sha256", "inode")}),
            ("the page and the Domains row name the same state after the refusal",
             page["body"].get("state") == (rows.get(DOMAIN_A) or {}).get("state"),
             {"page": {k: page["body"].get(k) for k in page_keys}, "row": rows.get(DOMAIN_A),
              "ledger": {k: ledger.get(k) for k in ("state", "state_reason")}})]
        self.current["b4"] = {"take": taken["entry"], "page": {k: page["body"].get(k) for k in page_keys}, "row": rows.get(DOMAIN_A),
                              "ledger": {k: ledger.get(k) for k in ("state", "state_reason", "decision", "backup_path")},
                              "backups_after": [x.get("path") for x in after["side"]["backups"]],
                              "a_lines": s10.site_lines(window["by_unit"].get("celikpanel-panel.service") or [], DOMAIN_A),
                              "browser": browser, "unlocked": (unlocked.get("after") or {}).get("lsattr")}
        self.judge("B4", checks, self.current["b4"])
        self.restore10("b4-end", DOMAIN_A)

    # -- the browser windows (set7's handover; only with SET11_HAND) ----------------------------------------------

    def restart_panel(self, verb: str) -> dict:
        before = time.time()
        script = (f"date -u +%FT%T.%NZ; systemctl show celikpanel-panel.service -p MainPID -p ActiveState\n"
                  f"systemctl {verb} celikpanel-panel.service; echo rc=$?\n"
                  f"date -u +%FT%T.%NZ; systemctl show celikpanel-panel.service -p MainPID -p ActiveState "
                  f"-p ActiveEnterTimestamp -p InactiveEnterTimestamp\n")
        out = self.guest(script, timeout=180).stdout
        return {"what": f"systemctl {verb} celikpanel-panel.service on the disposable guest, by the harness",
                "host_before": before, "host_after": time.time(), "guest_output": out.strip()}

    def browser_window(self, name: str, wait_seconds: int, extra: dict) -> dict:
        prefix = os.environ.get("SET11_HAND")
        if not prefix:
            return {"window": name, "skipped": "no browser in this lab (SET11_HAND is not set)"}
        hand = HAND_ROOT / f"{prefix}-{name}"
        if hand.exists():
            return {"window": name, "skipped": f"the handover directory {hand} exists; a window is never reused"}
        hand.mkdir(parents=True, mode=0o755)
        SECRET_ROOT.mkdir(parents=True, mode=0o700, exist_ok=True)
        os.chmod(SECRET_ROOT, 0o700)
        secret = SECRET_ROOT / f"{prefix}-{name}.json"
        leaf = self.refresh_pin()
        handover = {"schema": "celikpanel/set11-handover/v1", "window": name, "local_port": self.local_port,
                    "base": f"https://127.0.0.1:{self.local_port}", "username": self.state.get("username"),
                    "tls_leaf_sha256": leaf, "at": base.utc_now(), "password_file": str(secret)} | extra
        descriptor = os.open(secret, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as handle:
            json.dump({"username": self.state.get("username"), "password": self._password,
                       "redact": [base.ACCEPTANCE_FIXTURE_KEY]}, handle)
        (hand / "ready.json").write_text(json.dumps(handover, indent=2, sort_keys=True) + "\n")
        print(json.dumps({"set11": "ready", "window": name, "hand": str(hand), "port": self.local_port}), flush=True)
        deadline = time.monotonic() + wait_seconds
        requests, ensured, done = [], time.monotonic(), None
        try:
            while time.monotonic() < deadline:
                for request in sorted(hand.glob("restart-request-*")):
                    number = request.name.rsplit("-", 1)[-1]
                    marker = hand / f"restart-done-{number}.json"
                    if marker.exists():
                        continue
                    verb = next((v for v in ("stop", "start", "restart") if number.startswith(v)), "restart")
                    record = self.restart_panel(verb) | {"request": number}
                    requests.append(record)
                    marker.write_text(json.dumps(record, indent=2, sort_keys=True) + "\n")
                if (hand / "done.json").exists():
                    done = json.loads((hand / "done.json").read_text() or "{}")
                    break
                if time.monotonic() - ensured > 10:
                    self.tunnel.ensure()
                    ensured = time.monotonic()
                time.sleep(1)
        finally:
            try:
                secret.unlink()
            except FileNotFoundError:
                pass
        record = {"window": name, "hand": str(hand), "requests": requests, "browser_done": done,
                  "password_file_removed": not secret.exists(), "ended_at": base.utc_now()}
        if done is None:
            record["not_finished"] = f"the browser did not finish within {wait_seconds} s"
        return record

    def final_window(self, checks: dict) -> str:
        record = self.browser_window("d-e", 4 * 3600, {"cells": "D and E"})
        checks.update(record)
        self.record_json("browser-window.json", record)
        if record.get("skipped"):
            return "skipped"
        if record.get("not_finished"):
            raise base.StepInconclusive(record["not_finished"])
        self.ensure_session()
        return "observed"

    # -- the cell -----------------------------------------------------------------------------------------------

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("site", self.site, needs=("setup",))
        functions = {"S0-sites": self.s0_sites, "h-i-unchanged-start": self.h_i, "C-directory-modes": self.c_modes,
                     "A1-probe-ready": self.a1, "A4-nginx-stopped": self.a4, "A2-include-missing": self.a2,
                     "A5-include-back": self.a5, "A3-mail-name": self.a3, "A6-bound-address": self.a6,
                     "B1-locked-start": self.b1, "B2-unlocked-recheck": self.b2, "B3-save-writes": self.b3,
                     "B4-take-refused": self.b4}
        wanted = ARCH_SECTIONS if self.settings.node == "arch" else tuple(k for k, _ in SECTIONS_FULL)
        for key, title in SECTIONS_FULL:
            if key not in wanted:
                continue
            if key != "S0-sites" and not (self.s10.get("ref") and self.ids().get(DOMAIN_A)):
                self.step(key, lambda checks: checks.update(reason="S0 did not record the sites") or "not-run")
                continue
            self.section(key, title, functions[key], needs=("site",))
            if key != "S0-sites" and self.sections.get(key, {}).get("verdict") not in ("passed", "failed"):
                self.recover11(key)
        self.step("set11-browser-window", self.final_window, needs=("site",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None, "provenance": {"what": "set11: ead69cdcc installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set11-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "set11": {k: v for k, v in self.s10.items() if k != "ref"},
                  "sections": self.sections, "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set11: a measurement on the fresh install; no update is started and no P0 row is judged."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)

    def recover11(self, label: str) -> None:
        """Lab recovery after a harness error inside a section: nginx and the Panel started again if not active, A's
        immutable flag cleared."""
        try:
            self.snap11(f"{label}-lab-recover-chattr", "owner-chattr", domain=DOMAIN_A, flag="-i")
            if (self.snap11(f"{label}-lab-recover-acme", "read-acme", domains=[DOMAIN_A]).get("nginx_active")) != "active":
                self.snap11(f"{label}-lab-recover-nginx", "owner-nginx", action="start")
                self.current.setdefault("lab_preparation", []).append({"label": label, "what": "nginx started again"})
        except Exception as exc:  # noqa: BLE001
            self.current.setdefault("lab_preparation", []).append({"label": label, "recovery_error": type(exc).__name__})
        self.recover10(label)


# -- the update cell ----------------------------------------------------------------------------------------------

class Set11UpdateTrial(s10.Set10UpdateTrial):
    """set10's update sections over set5's start-check cell: the candidate's start check fails after the database
    publication and the update returns to alpha.81 by itself."""

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        if HELPER11 not in helpers:
            helpers[HELPER11] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER11, HELPER11)[1]
        return helpers

    def site_domains(self) -> list:
        return [s10.R1, s10.R2, s10.R3, "upd1-owner.test"]


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = {"schema": "celikpanel/set11-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES),
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation on the guest are recorded as such; no certificate authority and no "
                    "licence service is contacted by the driver"}
    if name in CELLS:
        settings = CELLS[name]
        wanted = ARCH_SECTIONS if settings.node == "arch" else tuple(k for k, _ in SECTIONS_FULL)
        plan.update(cell_kind=CELL_KIND, candidate={k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
                    setup={"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
                    steps=[s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup", "site"]
                          + list(wanted) + ["set11-browser-window", s6.PIN_END_STEP, "collect"])
    else:
        cell = base.validate_cell(name)
        plan.update(cell_kind=UPDATE_KIND, update=s6.build_plan(name, artifacts, work_root, local_port),
                    added_steps=[s10.PRE_STEP, s10.AFTER_STEP] + ([s10.RESTORE_STEP] if cell.variant == "good" else []))
    return plan


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("plan", "run"):
        cmd = sub.add_parser(name)
        cmd.add_argument("--cell", required=True, choices=sorted(CELLS) + sorted(UPDATE_CELLS))
        cmd.add_argument("--artifacts", required=True, type=Path)
        cmd.add_argument("--work-root", required=True)
        cmd.add_argument("--local-port", type=int, default=18443)
        if name == "plan":
            cmd.add_argument("--dry-run", action="store_true", help="validate the plan without any guest")
        else:
            cmd.add_argument("--execute", action="store_true")
    args = parser.parse_args(argv)
    base.validate_work_root(args.work_root)
    if not 1024 < args.local_port < 65536:
        parser.error("--local-port must be an unprivileged loopback port")
    document = json.loads(args.artifacts.read_text())
    base.configure_labels(document)
    cell = CELLS[args.cell].cell if args.cell in CELLS else base.validate_cell(args.cell)
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    if args.cell in CELLS:
        result = Set11Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
        print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                          "sections": {k: v.get("verdict") for k, v in result["sections"].items()}}, sort_keys=True))
    else:
        result = Set11UpdateTrial(cell, document, args.work_root, args.local_port).execute()
        print(json.dumps({"overall": result["overall"], "outcome": result["outcome"]["classification"],
                          "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
