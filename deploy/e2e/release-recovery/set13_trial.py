#!/usr/bin/env python3
"""set13: a short native round on commit 816b8dd60 (the corrections after the twelfth native record), on disposable
QEMU guests (lab.py): the code installed FRESH (set6's steps), set10's S0 (two PHP sites A ``set12-a.test``, B
``set12-b.test``) and h-i, then five cells, and a browser window on request (SET12_HAND):

  C1  pool take refused by ``php-fpm -t`` (set12's RP1 method: an owner's pool file elsewhere that the check refuses):
      502 SITE_CONFIG_SERVICE_REFUSED, restore confirmed, service not_reloaded, service_reason
      config_refused_elsewhere, service_active true, the check's line for an administrator; PHP-FPM still active (MainPID
      and NRestarts unchanged, no start-limit line); the owner's pool byte-equal. Then the owner removes their file and
      takes again: 200, PHP-FPM reloaded.
  C2  pool take while the owner has stopped PHP-FPM (set12's RP2): restore confirmed, service not_running, nothing
      reloaded or started.
  C3  vhost take refused by nginx: (a) set10/12's method (an owner's include-directory file conflicts with CelikPanel's
      text only) -> the put-back passes nginx -t and nginx is reloaded (service reloaded); (b) an owner's file in
      /etc/nginx/conf.d that nginx refuses whatever the vhost -> not_reloaded, config_refused_elsewhere; nginx state.
  C4  the pool not read while B's pool save holds the Agent's pool lock (set12's U method, a 75 s reload drop-in): page
      unknown/not_read with read_error; the Domains row unknown, kind php_pool, reason, read_error; after the lock
      clears a read -> the row back.
  C5  a Node site (Node.js installed through the Panel) with a bare ``node`` start command: the unit's
      ``Environment=PATH=`` and ``ExecStart=/bin/sh -c``, the app runs (not 127); with logging shims: a plain take
      (what runs); a take whose daemon-reload fails once (lab injection) so the owner's unit is put back: verify passes
      -> try-restart (service reloaded); verify refuses (an owner's ExecStartPre= naming a missing program) -> not
      restarted (config_refused_elsewhere); systemd-analyze unavailable -> not restarted (check_unavailable).

The driver calls only the Panel's HTTP API; every native fact is read over SSH; owner actions and lab preparation on
the guest are recorded as such. Every result carries ``native_evidence: false``.

  set13_trial.py plan --cell set13-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set13_trial.py run  --cell set13-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import base64
import dataclasses
import json
import os
from pathlib import Path
import sys
import threading
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set12_trial as s12  # noqa: E402

s11, s10, s6, sw, base = s12.s11, s12.s10, s12.s6, s12.sw, s12.base
fx = s12.fx
DOMAIN_A, DOMAIN_B, DOMAIN_N = s12.DOMAIN_A, s12.DOMAIN_B, "set13-n.test"
HELPER13 = "guest_set13_native.py"
CELL_KIND = "set13-fresh-install"
CELLS = {
    "set13-debian13": sw.SettingsCell("set13-debian13", "debian13", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set13-ubuntu": sw.SettingsCell("set13-ubuntu", "ubuntu", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set13-arch": sw.SettingsCell("set13-arch", "arch", "web", sw.WEB_PRESET + ("postgresql",), False),
}
SECTIONS = (
    ("S0-sites", "two PHP sites; header, digest, ledger row, include directory, nginx -T (set10's section)"),
    ("h-i-unchanged-start", "a restart with every site unchanged (set10's section)"),
    ("C1-pool-check-refused", "pool take refused by php-fpm -t: put back, not reloaded; then the owner's fix and a take"),
    ("C2-pool-php-stopped", "pool take while the owner stopped PHP-FPM: put back, not running, nothing reloaded"),
    # C4 and C5 before C3 (set13 Debian run a: a refused vhost take leaves the vhost's ledger row "foreign /
    # nginx_refused" until the next write, and the Domains row shows that row instead of the unread pool).
    ("C4-pool-not-read", "the pool not read: the page and the Domains row"),
    ("C5-app-unit", "the Node unit's PATH and /bin/sh -c; verify before try-restart after a put-back"),
    ("C3-vhost-nginx-refused", "vhost take refused by nginx: put back; reloaded or not by nginx -t's answer"),
)
STALL_SECONDS = 12  # after a refused take: long enough for a failing PHP-FPM to reach systemd's start limit
FAIL_LINES = ("Start request repeated too quickly", "Main process exited", "Failed with result")
NODE_COMMAND = "node -e \"require('http').createServer((q,s)=>s.end('%s')).listen(process.env.PORT)\""


def body_of(answer: dict) -> dict:
    return answer.get("body") or {}


def service_fields(body: dict) -> dict:
    keep = ("code", "reason", "detail", "restore", "service", "service_reason", "service_active", "vars", "details", "error")
    return {k: body.get(k) for k in keep if k in body}


def lines_with(lines: list, needles: tuple, since: float | None = None) -> list:
    return [l for l in lines or [] if any(n in l for n in needles)
            and (since is None or (s10.journal_epoch(l) or 0) >= since)]


def calls_of(lines: list, comm: str | None = None) -> list:
    """The shim log's lines (optionally of one caller name)."""
    return [l for l in lines or [] if comm is None or f"comm={comm}" in l or "injected-failure" in l or " rc=" in l]


class Set13Trial(s12.Set12Trial):
    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[HELPER13] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER13, HELPER13)[1]
        return helpers

    @property
    def s13(self) -> dict:
        return self.state.setdefault("set13", {})

    def native13(self, mode: str, **arguments: Any) -> dict:
        payload = base64.b64encode(json.dumps(arguments).encode()).decode()
        return self.helper(HELPER13, mode, "--args-b64", payload, timeout=420)

    def snap13(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.native13(mode, **arguments)
        self.native_sequence += 1
        name = f"native/{self.native_sequence:03d}-{s10.safe_name(label)}.json"
        self.record_json(name, value)
        self.current.setdefault("natives", []).append({"label": label, "mode": mode, "file": name,
                                                       "owner_action": mode.startswith("owner-")})
        value["_file"] = f"{self.step_dir}/{name}"
        return value

    def owner13(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.snap13("owner-" + label, mode, **arguments)
        self.current.setdefault("owner_actions", []).append({"label": label, "mode": mode, "at": value.get("at")})
        return value

    def lab13(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.snap13("lab-" + label, mode, **arguments)
        self.current.setdefault("lab_preparation", []).append({"label": label, "mode": mode, "file": value["_file"]})
        return value

    # -- C1 / C2: pool take refused -----------------------------------------------------------------------------

    def php_unit_lines(self, window: dict, unit: str, since: float) -> list:
        return [l for l in (window["by_unit"].get(unit + ".service") or []) + (window.get("systemd_all") or [])
                if (s10.journal_epoch(l) or 0) >= since and (unit in l or "php" in l.lower())]

    def c1_pool_check_refused(self) -> None:
        a_id = self.a_id()
        self.restore_pool("c1")
        edit = self.owner12("c1-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
        shown = self.sc("c1 page (owner-edited pool)", a_id)
        pool = s12.file_of(shown["body"], "php_pool")
        before = self.pool12("c1-0-before", DOMAIN_A)
        _, p0 = self.the_pool(before)
        cause = self.owner12("c1-owner-pool-file-refused", "owner-broken-pool", domain=DOMAIN_A)
        unit = before.get("php_unit") or ""
        u0 = cause.get("php_unit") or before.get("php_unit_state") or {}
        since = self.guest_epoch()
        answer = self.take_post("c1 take on the owner's pool (php-fpm -t refuses another file)", a_id, pool, "php_pool")
        time.sleep(STALL_SECONDS)
        after = self.pool12("c1-1-after", DOMAIN_A)
        _, p1 = self.the_pool(after)
        window = self.window12("c1", since - 1)
        lines = self.php_unit_lines(window, unit, since)
        u1 = after.get("php_unit_state") or {}
        b = body_of(answer)
        line = ((b.get("vars") or {}).get("line") or "")
        test_rc = (cause.get("php_fpm_test") or {}).get("returncode")
        checks = [
            ("the cause: the owner's pool file makes php-fpm -t fail (exit status recorded)", test_rc not in (0, None),
             {"returncode": test_rc, "stderr": str((cause.get("php_fpm_test") or {}).get("stderr"))[-400:]}),
            ("502 SITE_CONFIG_SERVICE_REFUSED, reason php_pool", answer["status"] == 502 and b.get("code") == "SITE_CONFIG_SERVICE_REFUSED"
             and b.get("reason") == "php_pool", answer["entry"]),
            ("restore confirmed", b.get("restore") == "confirmed", service_fields(b)),
            ("service not_reloaded, service_reason config_refused_elsewhere", b.get("service") == "not_reloaded"
             and b.get("service_reason") == "config_refused_elsewhere", service_fields(b)),
            ("service_active true", b.get("service_active") == "true", b.get("service_active")),
            ("for an administrator: the check's line in vars.line and in details, and in the sentence",
             bool(line) and line in (b.get("details") or []) and line in str(b.get("error") or ""),
             {"vars": b.get("vars"), "details": b.get("details"), "error": b.get("error")}),
            ("PHP-FPM still active afterwards: MainPID and NRestarts unchanged",
             u1.get("ActiveState") == "active" and u1.get("MainPID") == u0.get("MainPID") and u1.get("NRestarts") == u0.get("NRestarts"),
             {"before": u0, "after": u1}),
            ("no reload of PHP-FPM after the take and no start-limit / exit line in its journal",
             not lines_with(lines, ("Reloading", "Reloaded") + FAIL_LINES), lines[-12:]),
            ("the owner's pool is byte-equal (sha256, mode, uid, gid)",
             (p1.get("sha256"), p1.get("mode"), p1.get("uid"), p1.get("gid")) == (p0.get("sha256"), p0.get("mode"), p0.get("uid"), p0.get("gid"))
             and p1.get("sha256") == (edit.get("after") or {}).get("sha256"),
             {k: [p0.get(k), p1.get(k)] for k in ("sha256", "inode", "mode", "uid", "gid")}),
        ]
        # The owner removes their file (no reload of their own) and takes again.
        removed = self.owner12("c1-owner-pool-file-removed", "owner-broken-pool", domain=DOMAIN_A, remove=True)
        shown2 = self.sc("c1 page after the owner's fix", a_id)
        pool2 = s12.file_of(shown2["body"], "php_pool")
        since2 = self.guest_epoch()
        taken = self.take_post("c1 take after the owner removed their file", a_id, pool2, "php_pool")
        time.sleep(s10.SETTLE)
        after2 = self.pool12("c1-2-after-the-second-take", DOMAIN_A)
        _, p2 = self.the_pool(after2)
        window2 = self.window12("c1-second-take", since2 - 1)
        lines2 = self.php_unit_lines(window2, unit, since2)
        t = body_of(taken)
        checks += [
            ("after the owner's fix php-fpm -t passes", (removed.get("php_fpm_test") or {}).get("returncode") == 0, removed.get("php_fpm_test")),
            ("the second take answers 200 taken; CelikPanel's pool text with a valid header",
             taken["status"] == 200 and t.get("outcome") == "taken" and bool((p2.get("header") or {}).get("valid")), taken["entry"]),
            ("PHP-FPM was reloaded after the second take (journal) and is active",
             bool(lines_with(lines2, ("Reloading", "Reloaded"))) and (after2.get("php_unit_state") or {}).get("ActiveState") == "active",
             {"lines": lines2[-8:], "unit": after2.get("php_unit_state")}),
        ]
        self.current["second_take_fields"] = {k: t.get(k) for k in t if k not in ("diff",)}
        self.judge("C1", checks, {"journal": window["_file"], "journal_second": window2["_file"],
                                  "backups_after": [x.get("path") for x in (p1.get("side_files") or {}).get("backups") or []]})
        self.restore_pool("c1-end")

    def c2_pool_php_stopped(self) -> None:
        a_id = self.a_id()
        self.restore_pool("c2")
        edit = self.owner12("c2-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
        shown = self.sc("c2 page (owner-edited pool)", a_id)
        pool = s12.file_of(shown["body"], "php_pool")
        before = self.pool12("c2-0-before", DOMAIN_A)
        _, p0 = self.the_pool(before)
        unit = before.get("php_unit") or ""
        stopped = self.owner12("c2-php-stop", "owner-php12", domain=DOMAIN_A, action="stop")
        since = self.guest_epoch()
        answer = self.take_post("c2 take on the owner's pool while PHP-FPM is stopped", a_id, pool, "php_pool")
        time.sleep(s10.SETTLE)
        after = self.pool12("c2-1-after", DOMAIN_A)
        _, p1 = self.the_pool(after)
        window = self.window12("c2", since - 1)
        lines = self.php_unit_lines(window, unit, since)
        b = body_of(answer)
        u1 = after.get("php_unit_state") or {}
        checks = [
            ("the cause: PHP-FPM stopped by the owner", (stopped.get("after") or {}).get("ActiveState") in ("inactive", "failed"), stopped.get("after")),
            ("502 SITE_CONFIG_SERVICE_REFUSED, reason php_pool, restore confirmed",
             answer["status"] == 502 and b.get("code") == "SITE_CONFIG_SERVICE_REFUSED" and b.get("reason") == "php_pool"
             and b.get("restore") == "confirmed", answer["entry"]),
            ("service not_running, service_active false", b.get("service") == "not_running" and b.get("service_active") == "false", service_fields(b)),
            ("the sentence says PHP-FPM was not running and was neither reloaded nor started",
             "not running" in str(b.get("error") or ""), b.get("error")),
            ("nothing reloaded or started: PHP-FPM still inactive, no Reloading/Starting line after the take",
             u1.get("ActiveState") in ("inactive", "failed") and not lines_with(lines, ("Reloading", "Reloaded", "Starting", "Started")),
             {"unit_after": u1, "lines": lines[-10:]}),
            ("the owner's pool is byte-equal (sha256, mode, uid, gid)",
             (p1.get("sha256"), p1.get("mode"), p1.get("uid"), p1.get("gid")) == (p0.get("sha256"), p0.get("mode"), p0.get("uid"), p0.get("gid"))
             and p1.get("sha256") == (edit.get("after") or {}).get("sha256"),
             {k: [p0.get(k), p1.get(k)] for k in ("sha256", "inode", "mode", "uid", "gid")}),
        ]
        self.judge("C2", checks, {"journal": window["_file"]})
        self.owner12("c2-php-start", "owner-php12", domain=DOMAIN_A, action="start")
        self.restore_pool("c2-end")

    # -- C3: vhost -------------------------------------------------------------------------------------------------

    def nginx_lines(self, window: dict, since: float) -> list:
        return [l for l in (window["by_unit"].get("nginx.service") or []) + (window.get("systemd_manager") or [])
                if (s10.journal_epoch(l) or 0) >= since and "nginx" in l]

    def c3_vhost(self) -> None:
        a_id = self.a_id()
        # (a) set10/12's RN method
        self.restore10("c3a", DOMAIN_A)
        self.owner10("c3a-replace", "owner-vhost10", domain=DOMAIN_A, action="replace", reload=True)
        put = self.owner10("c3a-include", "owner-include", domain=DOMAIN_A, name="set10-dup-root.conf", kind="duplicate-root", reload=True)
        before = self.read10("c3a-0-before", domains=[DOMAIN_A])
        n0 = self.snap13("c3a-nginx-before", "read-nginx13")
        shown = self.sc("c3a page", a_id)
        since = self.guest_epoch()
        answer = self.take_post("c3a take while an owner's include-directory file conflicts with CelikPanel's text", a_id, shown["body"], None)
        time.sleep(s10.SETTLE)
        after = self.read10("c3a-1-after", domains=[DOMAIN_A])
        n1 = self.snap13("c3a-nginx-after", "read-nginx13")
        http = self.http10("c3a-after", DOMAIN_A, "/")
        window = self.window12("c3a", since - 1)
        b = body_of(answer)
        vb, va = before["sites"][DOMAIN_A]["vhost"], after["sites"][DOMAIN_A]["vhost"]
        nl = self.nginx_lines(window, since)
        checks = [
            ("(a) the owner's conflicting include file passed nginx -t with the owner's vhost", (put.get("nginx_test") or {}).get("returncode") == 0, put.get("nginx_test")),
            ("(a) 502 SITE_CONFIG_NGINX_REFUSED, restore confirmed", answer["status"] == 502 and b.get("code") == "SITE_CONFIG_NGINX_REFUSED"
             and b.get("restore") == "confirmed", answer["entry"]),
            ("(a) the put-back passes nginx -t: service reloaded, service_active true", b.get("service") == "reloaded" and b.get("service_active") == "true",
             service_fields(b)),
            ("(a) nginx reloaded after the take (journal) and active; nginx -t passes", bool(lines_with(nl, ("Reloading", "Reloaded")))
             and (n1.get("unit") or {}).get("ActiveState") == "active" and (n1.get("nginx_test") or {}).get("returncode") == 0,
             {"lines": nl[-8:], "unit": [n0.get("unit"), n1.get("unit")]}),
            ("(a) the owner's vhost is byte-equal and served", (va.get("sha256"), va.get("mode"), va.get("uid"), va.get("gid"))
             == (vb.get("sha256"), vb.get("mode"), vb.get("uid"), vb.get("gid")) and "owner-replaced" in str(http.get("body") or ""),
             {k: [vb.get(k), va.get(k)] for k in ("sha256", "inode", "mode")} | {"body": str(http.get("body") or "")[:80]}),
        ]
        self.owner10("c3a-include-back", "owner-include", domain=DOMAIN_A, name="set10-dup-root.conf", remove=True, reload=False)
        self.restore10("c3a-end", DOMAIN_A)
        # (b) an owner's file nginx refuses whatever the vhost
        self.owner10("c3b-replace", "owner-vhost10", domain=DOMAIN_A, action="replace", reload=True)
        before = self.read10("c3b-0-before", domains=[DOMAIN_A])
        shown = self.sc("c3b page", a_id)
        cause = self.owner13("c3b-owner-conf-d-file", "owner-nginx13")
        n0 = self.snap13("c3b-nginx-before", "read-nginx13")
        since = self.guest_epoch()
        answer = self.take_post("c3b take while an owner's conf.d file makes nginx -t fail", a_id, shown["body"], None)
        time.sleep(s10.SETTLE)
        after = self.read10("c3b-1-after", domains=[DOMAIN_A])
        n1 = self.snap13("c3b-nginx-after", "read-nginx13")
        http = self.http10("c3b-after", DOMAIN_A, "/")
        window = self.window12("c3b", since - 1)
        b = body_of(answer)
        vb, va = before["sites"][DOMAIN_A]["vhost"], after["sites"][DOMAIN_A]["vhost"]
        nl = self.nginx_lines(window, since)
        line = (b.get("vars") or {}).get("line") or ""
        u0, u1 = n0.get("unit") or {}, n1.get("unit") or {}
        checks += [
            ("(b) the cause: nginx -t fails because of the owner's conf.d file", (cause.get("nginx_test") or {}).get("returncode") not in (0, None),
             {"returncode": (cause.get("nginx_test") or {}).get("returncode"), "stderr": str((cause.get("nginx_test") or {}).get("stderr"))[-300:]}),
            ("(b) 502 SITE_CONFIG_NGINX_REFUSED, restore confirmed", answer["status"] == 502 and b.get("code") == "SITE_CONFIG_NGINX_REFUSED"
             and b.get("restore") == "confirmed", answer["entry"]),
            ("(b) service not_reloaded, service_reason config_refused_elsewhere, service_active true",
             b.get("service") == "not_reloaded" and b.get("service_reason") == "config_refused_elsewhere" and b.get("service_active") == "true",
             service_fields(b)),
            ("(b) the check's line (vars.line) names the owner's file, also in details", "zz-set13-owner.conf" in line and line in (b.get("details") or []),
             {"vars": b.get("vars"), "details": b.get("details")}),
            ("(b) nginx not reloaded (no Reload line), still active with the same MainPID", not lines_with(nl, ("Reloading", "Reloaded"))
             and u1.get("ActiveState") == "active" and u1.get("MainPID") == u0.get("MainPID"), {"lines": nl[-8:], "unit": [u0, u1]}),
            ("(b) the owner's vhost is byte-equal and still served", (va.get("sha256"), va.get("mode"), va.get("uid"), va.get("gid"))
             == (vb.get("sha256"), vb.get("mode"), vb.get("uid"), vb.get("gid")) and "owner-replaced" in str(http.get("body") or ""),
             {k: [vb.get(k), va.get(k)] for k in ("sha256", "inode", "mode")} | {"body": str(http.get("body") or "")[:80]}),
        ]
        self.owner13("c3b-owner-conf-d-file-removed", "owner-nginx13", remove=True)
        self.restore10("c3b-end", DOMAIN_A)
        # Observation (not judged): with CelikPanel's text back (lab), the page and the Domains row; then the owner saves
        # the General settings unchanged (a write of the vhost) and the row is read again.
        page = self.sc("c3 end: page with CelikPanel's text back (lab)", a_id)
        rows = self.domains_rows("c3 end: Domains list before a write")
        ledger = [r for r in self.ledger_rows("c3-end", a_id) if r.get("kind") == "nginx_vhost"]
        saved = self.general_save10("c3 end: General settings saved unchanged", a_id)
        rows2 = self.domains_rows("c3 end: Domains list after the save")
        ledger2 = [r for r in self.ledger_rows("c3-end-after-save", a_id) if r.get("kind") == "nginx_vhost"]
        self.current["observation_vhost_row_after_refused_take"] = {
            "page_state": page["body"].get("state"), "row_before_write": rows.get(DOMAIN_A),
            "ledger_before_write": [{k: r.get(k) for k in ("state", "state_reason", "file_sha256", "observed_at")} for r in ledger[-1:]],
            "general_save": saved, "row_after_save": rows2.get(DOMAIN_A),
            "ledger_after_save": [{k: r.get(k) for k in ("state", "state_reason", "file_sha256", "observed_at")} for r in ledger2[-1:]]}
        self.judge("C3", checks, {"journal": window["_file"], "observation": self.current["observation_vhost_row_after_refused_take"]})

    # -- C4: unread pool ---------------------------------------------------------------------------------------------

    def hold_pool_lock(self, label: str) -> dict:
        """B's pool save while PHP-FPM's reload waits 75 s (lab drop-in), and A's page read meanwhile."""
        a_id, b_id = self.a_id(), self.ids()[DOMAIN_B]
        # An answered read first (set13 Ubuntu run a: after a lab put-back the pool's row still held the refused take's
        # owner_edited, which an unread observation rightly does not replace): the row starts from what is on disk.
        first = self.sc(f"{label} page of A before the hold (an answered read)", a_id)
        rows_before = self.domains_rows(f"{label} before the hold")
        slow = self.lab12(f"{label}-slow-reload", "lab-slow-reload", domain=DOMAIN_A, seconds=75)
        php = self.call(f"{label}: B's PHP settings read", "GET", f"/api/v1/domains/{b_id}/php")
        version = (php["_parsed"] or {}).get("php_version") if isinstance(php["_parsed"], dict) else None
        holder: dict = {}
        children = 7 if label == "c4" else 8

        def save_b() -> None:
            started = time.time()
            try:
                holder["save"] = self.call(f"{label}: B's pool save (holds the Agent's pool lock during the slow reload)", "POST",
                                           f"/api/v1/domains/{b_id}/php/pool",
                                           {"version": version, "pool_config": {"pm": "dynamic", "pm_max_children": children, "pm_start_servers": 2,
                                                                                "pm_min_spare_servers": 1, "pm_max_spare_servers": 3,
                                                                                "pm_max_requests": 500}}, timeout=400)
            except Exception as exc:  # noqa: BLE001
                holder["error"] = self.error_text(exc)
            holder["seconds"] = round(time.time() - started, 1)
        since = self.guest_epoch()
        thread = threading.Thread(target=save_b, daemon=True)
        thread.start()
        time.sleep(6)
        started = time.time()
        shown = self.sc(f"{label} page of A while B's pool save holds the lock", a_id)
        read_seconds = round(time.time() - started, 1)
        rows = self.domains_rows(f"{label} while the lock is held")
        ledger = [r for r in self.ledger_rows(f"{label}-during", a_id) if r.get("kind") == "php_pool"]
        thread.join(timeout=300)
        rows_after_save = self.domains_rows(f"{label} after B's save answered (no read of A since)")
        window = self.window12(label, since - 1)
        self.lab12(f"{label}-slow-reload-removed", "lab-slow-reload", domain=DOMAIN_A, remove=True)
        return {"first": first, "rows_before": rows_before, "slow": slow, "save": holder.get("save") or {}, "save_seconds": holder.get("seconds"), "save_error": holder.get("error"),
                "shown": shown, "read_seconds": read_seconds, "rows": rows, "ledger": ledger, "rows_after_save": rows_after_save,
                "window": window}

    def c4_pool_not_read(self) -> None:
        a_id = self.a_id()
        self.restore_pool("c4")
        held = self.hold_pool_lock("c4")
        body = held["shown"]["body"]
        pool = s12.file_of(body, "php_pool")
        row = held["rows"].get(DOMAIN_A) or {}
        row2 = held["rows_after_save"].get(DOMAIN_A) or {}
        panel = held["window"]["by_unit"].get("celikpanel-panel.service") or []
        again = self.sc("c4 page of A after the lock was released", a_id)
        rows3 = self.domains_rows("c4 after the read")
        ledger3 = [r for r in self.ledger_rows("c4-after", a_id) if r.get("kind") == "php_pool"]
        row3 = rows3.get(DOMAIN_A)
        self.judge("C4", [
            ("before the hold an answered read shows the pool managed_unchanged and the row managed",
             s12.file_of(held["first"]["body"], "php_pool").get("state") == "managed_unchanged"
             and (held["rows_before"].get(DOMAIN_A) or {}).get("state") in ("managed_unchanged", None),
             {"pool": s12.file_of(held["first"]["body"], "php_pool").get("state"), "row": held["rows_before"].get(DOMAIN_A)}),
            ("lab: PHP-FPM's reload waits 75 s; B's save answered after the page read",
             "sleep" in str(held["slow"].get("exec_reload")) and bool(held["save"]) and (held["save_seconds"] or 0) >= held["read_seconds"],
             {"exec_reload": held["slow"].get("exec_reload"), "save_status": held["save"].get("status"), "save_seconds": held["save_seconds"],
              "read_seconds": held["read_seconds"], "error": held["save_error"]}),
            ("the page: pool unknown, reason not_read, read_error given, action recheck; overall unknown/php_pool",
             held["shown"]["status"] == 200 and pool.get("state") == "unknown" and pool.get("reason") == "not_read" and bool(pool.get("read_error"))
             and body.get("overall_state") == "unknown" and body.get("overall_kind") == "php_pool",
             self.page_brief(body) | {"pool_read_error": pool.get("read_error"), "read_seconds": held["read_seconds"]}),
            ("the Panel's journal names the pool it could not read", any(DOMAIN_A in l and "php_pool" in l for l in panel),
             [l for l in panel if DOMAIN_A in l][:6]),
            ("the Domains row during the hold: state unknown, kind php_pool, reason not_read, read_error",
             row.get("state") == "unknown" and row.get("kind") == "php_pool" and row.get("reason") == "not_read" and bool(row.get("read_error")), row),
            ("the ledger row of the pool records unknown with the reason (no digest change)",
             bool(held["ledger"]) and held["ledger"][-1].get("state") == "unknown", held["ledger"][-1:]),
            ("the row stays unknown until a read answers (after B's save, before A is read again)", row2.get("state") == "unknown", row2),
            ("after the lock clears a read answers the pool managed_unchanged and the row is back",
             s12.file_of(again["body"], "php_pool").get("state") == "managed_unchanged" and (row3 or {}).get("state") in ("managed_unchanged", None),
             {"page": self.page_brief(again["body"]), "row": row3, "ledger": ledger3[-1:]}),
        ], {"journal": held["window"]["_file"]})

    # -- C5: application unit ----------------------------------------------------------------------------------------

    def unit13(self, label: str, since: float = 0, verify: bool = False) -> dict:
        value = self.snap13(label, "read-unit13", domain=DOMAIN_N, since_epoch=int(since), verify=verify)
        self.keep10(label, value.get("unit"))
        return value

    def lab_log(self, label: str, start: int) -> dict:
        return self.snap13(f"lab-log-{label}", "read-lab13", **{"from": start})

    def unit_take(self, label: str, n_id: int, since_offset: int, verify: bool = True) -> dict:
        shown = self.sc(f"{label} page", n_id)
        unitfile = s12.file_of(shown["body"], "app_unit")
        u0 = self.unit13(f"{label}-0-before", verify=verify)
        since = self.guest_epoch()
        taken = self.take_post(f"{label} take on the owner's unit (kind app_unit)", n_id, unitfile, "app_unit")
        time.sleep(6)
        u1 = self.unit13(f"{label}-1-after", since=since - 1)
        log = self.lab_log(label, since_offset)
        window = self.window12(label, since - 1)
        return {"page": unitfile, "u0": u0, "u1": u1, "answer": taken, "log": log, "window": window, "since": since}

    def c5_app_unit(self) -> None:
        node = self.install_node()
        version = node.get("version")
        if not version:
            self.judge("C5", [("a Node.js version installed through the Panel", None, node)])
            return
        n_id = self.create_php(DOMAIN_N)
        since0 = self.guest_epoch()
        first = self.call("c5: N becomes a Node site with a bare `node` start command", "PUT", f"/api/v1/domains/{n_id}/hosting",
                          {"project_type": "node", "start_command": NODE_COMMAND % "set13-n1", "runtime_version": version}, timeout=600)
        time.sleep(8)
        u0 = self.unit13("c5-0-unit", since=since0 - 1, verify=True)
        lines, state, text = u0.get("lines") or {}, u0.get("state") or {}, str((u0.get("unit") or {}).get("text") or "")
        node_dir = f"/opt/celikpanel/runtimes/node/{version}/bin"
        expected_path = f"Environment=PATH={node_dir}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
        journal0 = u0.get("journal") or []
        checks = [
            ("Node.js installed through the Panel (POST /api/v1/runtimes/node)", bool(version), node),
            ("the hosting change answers 200", first["status"] == 200, first.get("answer") or first["status"]),
            ("the unit carries Environment=PATH=<Node dir>:/usr/local/sbin:...:/bin", expected_path in text.splitlines()
             or any(l.replace('"', "") == expected_path for l in lines.get("environment_path") or []), lines.get("environment_path")),
            ("ExecStart=/bin/sh -c '…' (not -lc)", any(l.startswith("ExecStart=/bin/sh -c ") for l in lines.get("exec_start") or []),
             lines.get("exec_start")),
            ("the application runs: active, NRestarts 0, no exit 127 / 'not found' in its journal, it answers on its port",
             state.get("ActiveState") == "active" and state.get("NRestarts") in ("0", 0) and not lines_with(journal0, ("not found", "status=127"))
             and "set13-n1" in str((u0.get("http") or {}).get("body")),
             {"state": state, "http": u0.get("http"), "journal": journal0[-8:]}),
            ("the harness's own systemd-analyze verify of CelikPanel's unit (recorded)", True,
             {k: (u0.get("verify_by_harness") or {}).get(k) for k in ("returncode", "stdout", "stderr")}),
        ]
        # Logging shims for the rest (lab preparation).
        shims = self.lab13("c5-shims-install", "lab-shims13", action="install")
        start = int((shims.get("state") or {}).get("line_count") or 0)
        checks.append(("lab: the shims are in place and the Agent's PATH finds /usr/local/sbin first",
                       str((shims.get("state") or {}).get("agent_path") or "").split(":")[0] == "/usr/local/sbin",
                       {"agent_path": (shims.get("state") or {}).get("agent_path")}))
        # (b) a plain take on an owner-edited unit
        self.owner10("c5b-unit-line", "owner-unit10", domain=DOMAIN_N, action="append-directive")
        b = self.unit_take("c5b", n_id, start)
        start = int(b["log"].get("line_count") or start)
        agent_calls = [l for l in b["log"].get("lines") or [] if f"ppid={b['log'].get('agent_pid')}" in l]
        checks.append(("(b) plain take: 200 taken; the calls the Agent made are recorded (daemon-reload, try-restart; verify is "
                       "documented only after a put-back); MainPID changed, active",
                       b["answer"]["status"] == 200 and body_of(b["answer"]).get("outcome") == "taken"
                       and (b["u1"].get("state") or {}).get("MainPID") != (b["u0"].get("state") or {}).get("MainPID")
                       and (b["u1"].get("state") or {}).get("ActiveState") == "active",
                       {"answer": b["answer"]["entry"], "agent_calls": agent_calls,
                        "main_pid": [(b["u0"].get("state") or {}).get("MainPID"), (b["u1"].get("state") or {}).get("MainPID")]}))
        self.current["c5b_agent_calls"] = agent_calls
        # (c) a put-back whose unit passes verify
        edited = self.owner10("c5c-unit-line", "owner-unit10", domain=DOMAIN_N, action="append-directive")
        self.lab13("c5c-arm", "lab-shims13", action="arm")
        c = self.unit_take("c5c", n_id, start)
        start = int(c["log"].get("line_count") or start)
        cb = body_of(c["answer"])
        ccalls = [l for l in c["log"].get("lines") or [] if f"ppid={c['log'].get('agent_pid')}" in l or "injected-failure" in l]
        order = [w for l in ccalls for w in ("injected-failure", "daemon-reload", "verify", "try-restart") if w in l]
        checks += [
            ("(c) put-back after an injected daemon-reload failure: 502 SITE_CONFIG_SERVICE_REFUSED app_unit, restore confirmed",
             c["answer"]["status"] == 502 and cb.get("code") == "SITE_CONFIG_SERVICE_REFUSED" and cb.get("reason") == "app_unit"
             and cb.get("restore") == "confirmed", c["answer"]["entry"]),
            ("(c) verify passed -> service reloaded (try-restart), service_active true", cb.get("service") == "reloaded" and cb.get("service_active") == "true",
             service_fields(cb)),
            ("(c) the Agent's calls in order: failed daemon-reload, daemon-reload, systemd-analyze verify <unit>, try-restart",
             "injected-failure" in order and "verify" in order and "try-restart" in order and order.index("verify") < order.index("try-restart"),
             ccalls),
            ("(c) the owner's unit is back byte-equal; the application was restarted (MainPID changed) and is active",
             (c["u1"].get("unit") or {}).get("sha256") == (edited.get("after") or {}).get("sha256")
             and (c["u1"].get("state") or {}).get("MainPID") != (c["u0"].get("state") or {}).get("MainPID")
             and (c["u1"].get("state") or {}).get("ActiveState") == "active",
             {"sha256": [(edited.get("after") or {}).get("sha256"), (c["u1"].get("unit") or {}).get("sha256")],
              "main_pid": [(c["u0"].get("state") or {}).get("MainPID"), (c["u1"].get("state") or {}).get("MainPID")]}),
        ]
        # (d) a put-back whose unit verify refuses (the owner's ExecStartPre= names a missing program)
        missing = self.owner13("c5d-unit-missing-prestart", "owner-unit13", domain=DOMAIN_N, action="add-missing-prestart")
        self.lab13("c5d-arm", "lab-shims13", action="arm")
        d = self.unit_take("c5d", n_id, start)
        start = int(d["log"].get("line_count") or start)
        db = body_of(d["answer"])
        dcalls = [l for l in d["log"].get("lines") or [] if f"ppid={d['log'].get('agent_pid')}" in l or "injected-failure" in l or " rc=" in l]
        line = (db.get("vars") or {}).get("line") or ""
        s0, s1 = d["u0"].get("state") or {}, d["u1"].get("state") or {}
        checks += [
            ("(d) the cause: the harness's own verify of the owner's unit fails", (missing.get("verify_by_harness") or {}).get("returncode") not in (0, None),
             {k: (missing.get("verify_by_harness") or {}).get(k) for k in ("returncode", "stderr")}),
            ("(d) 502 SITE_CONFIG_SERVICE_REFUSED app_unit, restore confirmed, service not_reloaded, config_refused_elsewhere, service_active true",
             d["answer"]["status"] == 502 and db.get("code") == "SITE_CONFIG_SERVICE_REFUSED" and db.get("restore") == "confirmed"
             and db.get("service") == "not_reloaded" and db.get("service_reason") == "config_refused_elsewhere" and db.get("service_active") == "true",
             service_fields(db)),
            ("(d) for an administrator: verify's line (naming the missing program) in vars.line and details",
             "missing-prepare" in line and line in (db.get("details") or []), {"vars": db.get("vars"), "details": db.get("details")}),
            ("(d) the Agent ran verify, it failed, and no try-restart followed",
             any("verify" in l for l in dcalls) and not any("try-restart" in l for l in dcalls), dcalls),
            ("(d) the application was not restarted: MainPID and NRestarts unchanged, active; the owner's unit byte-equal",
             s1.get("MainPID") == s0.get("MainPID") and s1.get("NRestarts") == s0.get("NRestarts") and s1.get("ActiveState") == "active"
             and (d["u1"].get("unit") or {}).get("sha256") == (missing.get("after") or {}).get("sha256"),
             {"state": [s0, s1]}),
        ]
        # (e) systemd-analyze unavailable
        self.owner13("c5e-unit-missing-prestart-removed", "owner-unit13", domain=DOMAIN_N, action="remove-missing-prestart")
        hidden = self.lab13("c5e-hide-systemd-analyze", "lab-shims13", action="hide")
        self.lab13("c5e-arm", "lab-shims13", action="arm")
        e = self.unit_take("c5e", n_id, start, verify=False)
        start = int(e["log"].get("line_count") or start)
        unhidden = self.lab13("c5e-unhide-systemd-analyze", "lab-shims13", action="unhide")
        eb = body_of(e["answer"])
        ecalls = [l for l in e["log"].get("lines") or [] if f"ppid={e['log'].get('agent_pid')}" in l or "injected-failure" in l]
        s0, s1 = e["u0"].get("state") or {}, e["u1"].get("state") or {}
        checks += [
            ("(e) lab: systemd-analyze is not executable as the Agent sees it",
             (hidden.get("state") or {}).get("systemd_analyze_seen_by_agent", {}).get("executable") is False,
             {"seen_by_agent": (hidden.get("state") or {}).get("systemd_analyze_seen_by_agent"), "mount": hidden.get("mount")}),
            ("(e) 502, restore confirmed, service not_reloaded, service_reason check_unavailable, service_active true; the sentence names it",
             e["answer"]["status"] == 502 and eb.get("restore") == "confirmed" and eb.get("service") == "not_reloaded"
             and eb.get("service_reason") == "check_unavailable" and eb.get("service_active") == "true" and "systemd-analyze" in str(eb.get("error")),
             service_fields(eb)),
            ("(e) no try-restart; MainPID and NRestarts unchanged, active", not any("try-restart" in l for l in ecalls)
             and s1.get("MainPID") == s0.get("MainPID") and s1.get("NRestarts") == s0.get("NRestarts") and s1.get("ActiveState") == "active",
             {"calls": ecalls, "state": [s0, s1]}),
            ("(e) lab: systemd-analyze executable again", (unhidden.get("state") or {}).get("systemd_analyze_seen_by_agent", {}).get("executable") is True,
             (unhidden.get("state") or {}).get("systemd_analyze_seen_by_agent")),
        ]
        # cleanup: shims removed; a last take puts CelikPanel's text back and the application runs
        removed = self.lab13("c5-shims-remove", "lab-shims13", action="remove")
        shown = self.sc("c5 end page", n_id)
        last = self.take_post("c5 end: take CelikPanel's unit", n_id, s12.file_of(shown["body"], "app_unit"), "app_unit")
        time.sleep(6)
        u9 = self.unit13("c5-9-end")
        checks.append(("cleanup: shims removed; the last take answers 200 and the application answers",
                       not (removed.get("state") or {}).get("shims", {}).get("systemctl", {}).get("shim", {}).get("exists")
                       and last["status"] == 200 and "set13-n1" in str((u9.get("http") or {}).get("body")),
                       {"status": last["status"], "http": u9.get("http"), "state": u9.get("state")}))
        self.judge("C5", checks, {"node": node, "journals": [x["window"]["_file"] for x in (b, c, d, e)]})

    # -- browser preparations ------------------------------------------------------------------------------------

    def prep(self, name: str) -> dict:
        a_id = self.a_id()
        if name == "pool-edited":
            self.restore10("prep-pool", DOMAIN_A)
            self.restore_pool("prep-pool")
            self.owner12("prep-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
            body = self.sc("prep pool-edited read", a_id)["body"]
            return {"pool": s12.file_of(body, "php_pool").get("state"), "overall": body.get("overall_state")}
        if name == "broken-add":
            value = self.owner12("prep-broken-add", "owner-broken-pool", domain=DOMAIN_A)
            return {"test": (value.get("php_fpm_test") or {}).get("returncode"), "php": value.get("php_unit")}
        if name == "broken-remove":
            value = self.owner12("prep-broken-remove", "owner-broken-pool", domain=DOMAIN_A, remove=True)
            return {"test": (value.get("php_fpm_test") or {}).get("returncode"), "php": value.get("php_unit")}
        if name == "php-state":
            return {"pool": self.pool12(f"prep-php-state-{time.time_ns()}", DOMAIN_A).get("php_unit_state")}
        if name == "end":
            self.restore10("prep-end", DOMAIN_A)
            self.restore_pool("prep-end")
            return {"ok": True}
        if name == "u-unread":
            self.restore_pool("prep-u")
            held = self.hold_pool_lock("prep-u")
            return {"page_pool": {k: s12.file_of(held["shown"]["body"], "php_pool").get(k) for k in ("state", "reason", "read_error")},
                    "row": held["rows_after_save"].get(DOMAIN_A), "save_status": held["save"].get("status")}
        if name == "rows":
            return {"row": self.domains_rows("prep rows").get(DOMAIN_A)}
        return {"unknown": name}

    # -- the cell ---------------------------------------------------------------------------------------------------

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("site", self.site, needs=("setup",))
        functions = {"S0-sites": self.s0_sites, "h-i-unchanged-start": self.h_i, "C1-pool-check-refused": self.c1_pool_check_refused,
                     "C2-pool-php-stopped": self.c2_pool_php_stopped, "C3-vhost-nginx-refused": self.c3_vhost,
                     "C4-pool-not-read": self.c4_pool_not_read, "C5-app-unit": self.c5_app_unit}
        only = [k for k in os.environ.get("SET13_SECTIONS", "").split(",") if k]
        for key, title in SECTIONS:
            if only and key not in only and key not in ("S0-sites", "h-i-unchanged-start"):
                continue
            if key != "S0-sites" and not (self.s10.get("ref") and self.ids().get(DOMAIN_A)):
                self.step(key, lambda checks: checks.update(reason="S0 did not record the sites") or "not-run")
                continue
            self.section(key, title, functions[key], needs=("site",))
            if key != "S0-sites" and self.sections.get(key, {}).get("verdict") not in ("passed", "failed"):
                self.recover13(key)
        self.step("set13-browser-window", self.final_window12, needs=("site",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None, "provenance": {"what": "set13: 816b8dd60 installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set13-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "sections": self.sections, "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set13: a measurement on the fresh install; no update is started and no P0 row is judged."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)

    def recover13(self, label: str) -> None:
        for mode, arguments in (("lab-shims13", {"action": "unhide", "shim": False}), ("lab-shims13", {"action": "remove"}),
                                ("owner-nginx13", {"remove": True})):
            try:
                self.snap13(f"{label}-lab-recover-{mode}", mode, **arguments)
            except Exception as exc:  # noqa: BLE001
                self.current.setdefault("lab_preparation", []).append({"label": label, "mode": mode, "recovery_error": type(exc).__name__})
        self.recover12(label)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    settings = CELLS[name]
    return {"schema": "celikpanel/set13-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "cell_kind": CELL_KIND,
            "candidate": {k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
            "setup": {"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
            "steps": [s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup", "site"]
                     + [k for k, _ in SECTIONS] + ["set13-browser-window", s6.PIN_END_STEP, "collect"],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation on the guest are recorded as such; no certificate authority and no "
                    "licence service is contacted by the driver"}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("plan", "run"):
        cmd = sub.add_parser(name)
        cmd.add_argument("--cell", required=True, choices=sorted(CELLS))
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
    fx.configure(document)
    cell = CELLS[args.cell].cell
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    result = Set13Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                      "sections": {k: v.get("verdict") for k, v in result["sections"].items()}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
