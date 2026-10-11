#!/usr/bin/env python3
"""set12: the twelfth native record: D-031's second step (the PHP-FPM pool and the application unit) and the
corrections after the eleventh (the multi-file page and row, the restore fact, "apply", the addresses the validation
probe asks), on disposable QEMU guests (lab.py); and the owners' path, the update from the published alpha.82.

Two cell kinds, built on set11's and set10's drivers and changing none of their steps.

``set12-debian13`` | ``set12-ubuntu`` | ``set12-arch`` (``Set12Trial`` over ``set11_trial.Set11Trial``): the code
installed FRESH (set6's steps), set10's S0 (two PHP sites A ``set12-a.test``, B ``set12-b.test``) and h-i, then:

  M       A's pool: the owner adds a comment and a directive; a pool save; the page (files[], overall state); the
          Domains row; a PHP switch when a second version exists; take (dated copy, CelikPanel's text, PHP-FPM reloaded)
  U       the page while the Agent's pool lock is held by B's pool save (a lab drop-in makes PHP-FPM's reload take 75 s):
          the pool not read (state unknown, reason not_read), overall state, the Domains row
  RN      take on A's vhost refused by nginx (an owner's include-directory file): 502, restore, the previous file
  RP1     take on A's owner-edited pool while an owner's pool file elsewhere makes php-fpm -t fail: 502, restore
  RP2     the same take while the owner has stopped PHP-FPM (its reload fails): 502, restore
  P       apply: A locked at a start (kept, unwritable), unlocked, read (protection_removed, apply), POST apply; then B4:
          take on an owner-edited A locked after the read (409 UNWRITABLE, no dated copy, both facts)
  A       the probe's addresses: A's port-80 block bound to the guest's own address; to [::]:80 only; to an address
          the guest does not have (lab: ip_nonlocal_bind); nginx stopped
  N       a Node site (Node.js installed through the Panel): the unit's header, an owner drop-in across a change and a
          take, an owner edit refused (app_unit), take (dated copy, daemon-reload, try-restart)
  L       a third PHP site with an owner-edited pool deleted: the dated copy of the pool

``upd1-<platform>-good`` | ``upd1-<platform>-defective`` (``Set12UpdateTrial`` over ``set10_trial.Set10UpdateTrial``)
with an artifacts document whose baseline is the published v0.1.0-alpha.82 (set12_fixture.py, build-set12-
artifacts.sh): set6's update cells; before the update, on alpha.82: R2 and R3 (PHP) created, a Panel restart, R1
created, R3's vhost and R2's pool edited by the owner, a Node site RN when Node.js can be installed through the
Panel. After: the ledger, every vhost, pool and unit, the start lines, the site-config page of each site, the Domains
list, the four POST routes without a request identity. With an alpha.81 baseline document the cell is set10's
unchanged, plus the pool of each site read after the update.

The driver calls only the Panel's HTTP API; every native fact is read over SSH; owner actions and lab preparation on
the guest are recorded as such. Every result carries ``native_evidence: false``.

  set12_trial.py plan --cell set12-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set12_trial.py run  --cell set12-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
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
import threading
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set12_fixture as fx  # noqa: E402  (adds the alpha.82 baseline profile to owner_update_trial first)
import set11_trial as s11  # noqa: E402

s10, s8, s6, s4, sw, base = s11.s10, s11.s8, s11.s6, s11.s4, s11.sw, s11.base
DOMAIN_A, DOMAIN_B, DOMAIN_C, DOMAIN_N = "set12-a.test", "set12-b.test", "set12-c.test", "set12-n.test"
STATIC = s10.STATIC
HELPER12 = "guest_set12_native.py"
CELL_KIND, UPDATE_KIND = "set12-fresh-install", "set12-update"
RUN_ROOT = Path(os.environ.get("SET12_RUN_ROOT", "/var/tmp/cp-set12-run"))
s11.HAND_ROOT, s11.SECRET_ROOT = RUN_ROOT / "hand", RUN_ROOT / "secret"
CELLS = {
    "set12-debian13": sw.SettingsCell("set12-debian13", "debian13", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set12-ubuntu": sw.SettingsCell("set12-ubuntu", "ubuntu", "web_mail", sw.MAIL_PRESET + ("postgresql",), True),
    "set12-arch": sw.SettingsCell("set12-arch", "arch", "web", sw.WEB_PRESET + ("postgresql",), False),
}
UPDATE_CELLS = s10.UPDATE_CELLS
AVAILABLE = s11.AVAILABLE
PROBE_PATH = s11.PROBE_PATH
OWNER_POOL_DIRECTIVE = "php_admin_value[memory_limit] = 256M"
OWNER_POOL_COMMENT = "; set12: added by the server owner by hand"
INI_HEADER = "; celikpanel-render v2 sha256="
NONLOCAL_ADDRESS = "198.51.100.77"  # TEST-NET-2; the lab guests carry 192.0.2.x addresses of their own
NODE_FALLBACK = "22.20.0"
SECTIONS = (
    ("S0-sites", "two PHP sites; header, digest, ledger row, include directory, nginx -T (set10's section)"),
    ("h-i-unchanged-start", "a restart with every site unchanged (set10's section; it records the Panel's text)"),
    ("M-pool-owner-edit", "the owner's comment and directive in A's pool: save, page, row, switch, take"),
    ("U-pool-not-read", "the page while the Agent's pool lock is held: the pool not read"),
    ("RN-take-nginx-refused", "take refused by nginx: restore"),
    ("RP1-take-php-fpm-test-refused", "take on the pool refused by php-fpm -t: restore"),
    ("RP2-take-php-fpm-stopped", "take on the pool while PHP-FPM is stopped: restore"),
    ("P-apply-and-b4", "apply after a protection was removed; take refused on a file locked after the read"),
    ("A-probe-addresses", "the addresses the probe asks: own address, [::]:80 only, not this server's, nginx stopped"),
    ("N-app-unit", "a Node site's unit: header, drop-in, owner edit, take"),
    ("L-pool-delete", "deleting a site whose pool the owner edited"),
)


# -- pure rules (covered offline by test_set12_trial) ---------------------------------------------------------------

def name_the_sites() -> None:
    s8.DOMAIN_A, s8.DOMAIN_B = DOMAIN_A, DOMAIN_B
    s10.DOMAIN_A, s10.DOMAIN_B = DOMAIN_A, DOMAIN_B
    s11.DOMAIN_A, s11.DOMAIN_B = DOMAIN_A, DOMAIN_B


def file_of(body: dict, kind: str) -> dict:
    for item in (body or {}).get("files") or []:
        if isinstance(item, dict) and (item.get("kind") or "nginx_vhost") == kind:
            return item
    return {}


def sealed_ini(text: str | None) -> bool:
    return bool(text) and text.startswith(INI_HEADER)


def syn_destinations(rows: list) -> dict:
    """source port -> destination address of every SYN to port 80 the packet reader saw."""
    return {r.get("sport"): r.get("dst") for r in rows or [] if r.get("syn") and r.get("dport") == 80}


def journal_after(lines: list, epoch: float | None, needles: tuple) -> list:
    return [l for l in lines or [] if any(n in l for n in needles)
            and (epoch is None or (s10.journal_epoch(l) or 0) >= epoch)]


def first_state_line(rows: dict) -> dict:
    """Domains.tsx's first-state line from the Domains list rows: adopted = rows with adopted_from; left = rows whose
    state is unknown_origin; drawn only while one left-alone row has no decision for its bytes."""
    values = [v for v in rows.values() if isinstance(v, dict)]
    adopted = sum(1 for v in values if v.get("adopted_from"))
    left = sum(1 for v in values if v.get("state") == "unknown_origin")
    undecided = sum(1 for v in values if v.get("state") == "unknown_origin" and not v.get("kept_by_choice"))
    return {"adopted": adopted, "left": left, "drawn": undecided > 0}


class Set12Mixin:
    """Guest calls shared by both cell kinds."""

    def error_text(self, exc: Exception) -> str:
        error = self.redactor.text(f"{type(exc).__name__}: {exc}")[:1200]
        stderr = getattr(exc, "stderr", None)
        if stderr:
            error += " | guest stderr: " + (stderr if isinstance(stderr, str) else stderr.decode("utf-8", "replace"))[-600:]
        return error

    def native12(self, mode: str, **arguments: Any) -> dict:
        payload = base64.b64encode(json.dumps(arguments).encode()).decode()
        return self.helper(HELPER12, mode, "--args-b64", payload, timeout=420)

    def snap12(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.native12(mode, **arguments)
        self.native_sequence += 1
        name = f"native/{self.native_sequence:03d}-{s10.safe_name(label)}.json"
        self.record_json(name, value)
        self.current.setdefault("natives", []).append({"label": label, "mode": mode, "file": name,
                                                       "owner_action": mode.startswith("owner-")})
        value["_file"] = f"{self.step_dir}/{name}"
        return value

    def owner12(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.snap12("owner-" + label, mode, **arguments)
        self.current.setdefault("owner_actions", []).append({"label": label, "mode": mode, "at": value.get("at")})
        return value

    def lab12(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.snap12("lab-" + label, mode, **arguments)
        self.current.setdefault("lab_preparation", []).append({"label": label, "mode": mode, "file": value["_file"]})
        return value

    def window12(self, label: str, since: float) -> dict:
        return self.snap12(f"{label}-journal", "read-window12", since_epoch=int(since))

    def pool12(self, label: str, domain: str) -> dict:
        value = self.snap12(f"{label}-pool-{domain}", "read-pool12", domain=domain)
        for path, record in (value.get("pools") or {}).items():
            self.keep10(f"{label}-{domain}-{os.path.basename(path)}", record)
            pending = (record.get("side_files") or {}).get("pending") or {}
            self.keep10(f"{label}-{domain}-{os.path.basename(path)}.celikpanel-pending", pending)
            for index, backup in enumerate((record.get("side_files") or {}).get("backups") or []):
                self.keep10(f"{label}-{domain}-{os.path.basename(path)}.backup-{index}", backup)
        return value

    def the_pool(self, reading: dict) -> tuple:
        pools = reading.get("pools") or {}
        if len(pools) != 1:
            return None, {}
        return next(iter(pools.items()))

    def sc(self, label: str, domain_id: int) -> dict:
        """GET site-config with every file's difference kept."""
        entry = self.call(f"{label}: GET site-config", "GET", f"/api/v1/domains/{domain_id}/site-config", timeout=180)
        body = entry["_parsed"] if isinstance(entry["_parsed"], dict) else {}
        for item in body.get("files") or []:
            if isinstance(item, dict) and item.get("diff"):
                self.keep_text(f"files/{s10.safe_name(label)}-{item.get('kind') or 'nginx_vhost'}-diff.txt", item["diff"])
        if body.get("diff"):
            self.keep_text(f"files/{s10.safe_name(label)}-site-config-diff.txt", body["diff"])
        return {"status": entry["status"], "body": body}

    @staticmethod
    def page_brief(body: dict) -> dict:
        keep = ("kind", "state", "reason", "detail", "file_state", "system_error", "protection", "actions", "adopted_from",
                "path", "pending_path", "validation", "validation_unknown_reason", "validation_address", "pending_reason")
        return {"top": {k: body.get(k) for k in keep}, "overall_state": body.get("overall_state"),
                "overall_kind": body.get("overall_kind"),
                "files": [{k: f.get(k) for k in keep} | {"diff_lines": len((f.get("diff") or "").splitlines())}
                          for f in body.get("files") or [] if isinstance(f, dict)]}

    def ledger_rows(self, label: str, domain_id: int) -> list:
        reading = self.snap10(f"{label}-ledger", "read-ledger")
        rows = ((reading.get("ledger") or {}).get("managed_site_files") or [])
        return [r for r in rows if r.get("domain_id") == domain_id]

    def take_post(self, label: str, domain_id: int, file: dict, kind: str | None) -> dict:
        body = {"file_sha256": file.get("file_sha256"), "render_sha256": file.get("render_sha256")}
        if kind:
            body["kind"] = kind
        return self.post_raw(label, f"/api/v1/domains/{domain_id}/site-config/take", body, secrets.token_hex(16))


# ---------------------------------------------------------------------------
# The fresh-install cell
# ---------------------------------------------------------------------------

class Set12Trial(Set12Mixin, s11.Set11Trial):
    def __init__(self, *args: Any, **kwargs: Any) -> None:
        super().__init__(*args, **kwargs)
        name_the_sites()

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[HELPER12] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER12, HELPER12)[1]
        return helpers

    @property
    def s12(self) -> dict:
        return self.state.setdefault("set12", {})

    def site_domains(self) -> list:
        return [DOMAIN_A, DOMAIN_B, STATIC]

    def a_id(self) -> int:
        return self.ids()[DOMAIN_A]

    def pool_ref(self) -> dict:
        """CelikPanel's pool text of A as read the first time (lab restore source)."""
        if "pool_ref" not in self.s12:
            reading = self.pool12("ref", DOMAIN_A)
            path, record = self.the_pool(reading)
            if not path:
                raise base.StepFailed("A has no single pool file")
            self.s12["pool_ref"] = {"path": path, "text": record.get("text"), "uid": record.get("uid"), "gid": record.get("gid"),
                                    "mode": record.get("mode"), "sha256": record.get("sha256"), "header": record.get("header"),
                                    "php_unit": reading.get("php_unit")}
        return self.s12["pool_ref"]

    def restore_pool(self, label: str) -> dict:
        ref = self.pool_ref()
        return self.lab12(f"{label}-pool-restore", "lab-pool-restore", domain=DOMAIN_A, content_b64=s10.b64(ref["text"]),
                          uid=ref["uid"], gid=ref["gid"], file_mode=ref["mode"])

    # -- M --------------------------------------------------------------------------------------------------------

    def m_pool(self) -> None:
        a_id = self.a_id()
        ref = self.pool_ref()
        checks = [("A's pool carries the INI render header (\"; celikpanel-render v2 sha256=\") and its digest holds",
                   bool((ref.get("header") or {}).get("valid")), {"header": ref.get("header"), "path": ref["path"]})]
        edit = self.owner12("m-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
        edited = edit.get("after") or {}
        checks.append(("the owner's comment and directive pass php-fpm -t and PHP-FPM reloads",
                       (edit.get("php_fpm_test") or {}).get("returncode") == 0 and (edit.get("reload") or {}).get("returncode") == 0,
                       {"test": edit.get("php_fpm_test"), "reload": edit.get("reload")}))
        php = self.call("m: PHP settings read (GET /php)", "GET", f"/api/v1/domains/{a_id}/php")
        pbody = php["_parsed"] if isinstance(php["_parsed"], dict) else {}
        version = pbody.get("php_version")
        saved = self.call("m: pool save on the edited pool (POST /php/pool, pm_max_children 6)", "POST",
                          f"/api/v1/domains/{a_id}/php/pool",
                          {"version": version, "pool_config": {"pm": "dynamic", "pm_max_children": 6, "pm_start_servers": 2,
                                                               "pm_min_spare_servers": 1, "pm_max_spare_servers": 3,
                                                               "pm_max_requests": 500}}, timeout=300)
        sbody = saved["_parsed"] if isinstance(saved["_parsed"], dict) else {}
        r1 = self.pool12("m-1-after-the-save", DOMAIN_A)
        _, p1 = self.the_pool(r1)
        pending = (p1.get("side_files") or {}).get("pending") or {}
        checks += [
            ("the pool save answers 409 SITE_CONFIG_OWNER_EDITED, reason php_pool, detail owner_edited",
             saved["status"] == 409 and sbody.get("code") == "SITE_CONFIG_OWNER_EDITED" and sbody.get("reason") == "php_pool"
             and sbody.get("detail") == "owner_edited", saved.get("answer") or {"status": saved["status"], "body": sbody}),
            ("the owner's pool is kept byte for byte (sha256, inode, mtime)",
             (p1.get("sha256"), p1.get("inode"), p1.get("mtime_ns")) == (edited.get("sha256"), edited.get("inode"), edited.get("mtime_ns")),
             {k: [edited.get(k), p1.get(k)] for k in ("sha256", "inode", "mtime_ns", "mode", "uid", "gid")}),
            ("<pool>.celikpanel-pending holds CelikPanel's text (INI header) with the save's pm.max_children = 6",
             bool(pending.get("exists")) and sealed_ini(pending.get("text")) and "pm.max_children = 6" in str(pending.get("text")),
             {k: pending.get(k) for k in ("path", "exists", "sha256", "mode")})]
        shown = self.sc("m page after the save", a_id)
        body = shown["body"]
        pool = file_of(body, "php_pool")
        vhost = file_of(body, "nginx_vhost")
        rows = self.domains_rows("m after the save")
        diff = str(pool.get("diff") or "")
        checks += [
            ("GET site-config lists files[] = the vhost then the pool",
             shown["status"] == 200 and [f.get("kind") for f in body.get("files") or []] == ["nginx_vhost", "php_pool"],
             [f.get("kind") for f in body.get("files") or []]),
            ("the pool is owner_edited with keep/take/merge and its difference shows the owner's comment and directive",
             pool.get("state") == "owner_edited" and pool.get("actions") == ["keep", "take", "merge"]
             and OWNER_POOL_DIRECTIVE in diff and OWNER_POOL_COMMENT in diff,
             {"state": pool.get("state"), "actions": pool.get("actions"), "diff_lines": len(diff.splitlines()),
              "owner_lines_in_diff": [l for l in diff.splitlines() if "set12" in l or "memory_limit" in l]}),
            ("overall_state owner_edited, overall_kind php_pool (the vhost is managed_unchanged)",
             body.get("overall_state") == "owner_edited" and body.get("overall_kind") == "php_pool"
             and vhost.get("state") == "managed_unchanged", self.page_brief(body)),
            ("the Domains row shows the pool's state (owner_edited, kind php_pool)",
             (rows.get(DOMAIN_A) or {}).get("state") == "owner_edited" and (rows.get(DOMAIN_A) or {}).get("kind") == "php_pool",
             rows.get(DOMAIN_A))]
        others = [v for v in pbody.get("available_versions") or [] if v != version]
        if others:
            moved = self.call(f"m: PHP switch to {others[0]} on the edited pool", "POST", f"/api/v1/domains/{a_id}/php",
                              {"php_version": others[0]}, timeout=300)
            mbody = moved["_parsed"] if isinstance(moved["_parsed"], dict) else {}
            r2 = self.pool12("m-2-after-the-switch", DOMAIN_A)
            checks.append(("a PHP switch is refused (409, reason php_pool); the pool stays; nothing at the target version",
                           moved["status"] == 409 and mbody.get("reason") == "php_pool" and sorted(r2.get("pools") or {}) == [ref["path"]],
                           {"status": moved["status"], "reason": mbody.get("reason"), "pools": sorted(r2.get("pools") or {})}))
        else:
            checks.append(("a PHP switch on the edited pool is refused and writes nothing", None,
                           {"not_measured": "one PHP version on this guest", "php_version": version,
                            "available_versions": pbody.get("available_versions"), "programs": r1.get("php_fpm_programs")}))
        before_epoch = self.guest_epoch()
        unit_before = r1.get("php_unit_state") or {}
        taken = self.take_post("m take CelikPanel's pool text (kind php_pool)", a_id, pool, "php_pool")
        time.sleep(s10.SETTLE)
        r3 = self.pool12("m-3-after-take", DOMAIN_A)
        _, p3 = self.the_pool(r3)
        window = self.window12("m-take", before_epoch - 1)
        unit = r3.get("php_unit") or ""
        php_lines = journal_after((window["by_unit"].get(unit + ".service") or []) + window.get("systemd_all", []), before_epoch,
                                  ("Reload", "reload"))
        backups = (p3.get("side_files") or {}).get("backups") or []
        ledger = [r for r in self.ledger_rows("m-3", a_id) if r.get("kind") == "php_pool"]
        rows3 = self.domains_rows("m after take")
        checks += [
            ("take answers 200, outcome taken, with the backup path", taken["status"] == 200
             and taken["body"].get("outcome") == "taken" and bool(taken["body"].get("backup_path")), taken["entry"]),
            ("a dated <pool>.celikpanel-backup-<UTC> holds the owner's bytes",
             any(b.get("sha256") == edited.get("sha256") and ".celikpanel-backup-" in str(b.get("path")) for b in backups),
             [{k: b.get(k) for k in ("path", "sha256", "mode", "uid", "gid")} for b in backups]),
            ("the pool is now CelikPanel's text: INI header valid, no owner line, the held save's pm.max_children = 6",
             bool((p3.get("header") or {}).get("valid")) and OWNER_POOL_DIRECTIVE not in str(p3.get("text"))
             and "pm.max_children = 6" in str(p3.get("text")), {"header": p3.get("header"), "sha256": p3.get("sha256")}),
            ("mode, owner and group of the pool kept", (p3.get("mode"), p3.get("uid"), p3.get("gid")) == (edited.get("mode"), edited.get("uid"), edited.get("gid")),
             {k: [edited.get(k), p3.get(k)] for k in ("mode", "uid", "gid", "inode")}),
            ("PHP-FPM was reloaded after the take (a reload line in its journal; the unit active)",
             bool(php_lines) and (r3.get("php_unit_state") or {}).get("ActiveState") == "active",
             {"lines": php_lines[:8], "unit_before": unit_before, "unit_after": r3.get("php_unit_state")}),
            ("the ledger row of the pool records take_celikpanel and managed_unchanged",
             bool(ledger) and ledger[-1].get("decision") == "take_celikpanel" and ledger[-1].get("state") == "managed_unchanged",
             ledger[-1:] if ledger else None),
            ("the Domains row no longer names the pool", (rows3.get(DOMAIN_A) or {}).get("state") in ("managed_unchanged", None),
             rows3.get(DOMAIN_A))]
        self.s12["m_pool_after_take"] = p3.get("sha256")
        self.judge("M", checks, {"pool": ref["path"], "journal": window["_file"], "page": self.page_brief(body)})
        self.restore_pool("m-end")

    # -- U --------------------------------------------------------------------------------------------------------

    def u_not_read(self) -> None:
        a_id, b_id = self.a_id(), self.ids()[DOMAIN_B]
        self.restore_pool("u")
        slow = self.lab12("u-slow-reload", "lab-slow-reload", domain=DOMAIN_A, seconds=75)
        php = self.call("u: B's PHP settings read", "GET", f"/api/v1/domains/{b_id}/php")
        version = (php["_parsed"] or {}).get("php_version") if isinstance(php["_parsed"], dict) else None
        holder: dict = {}

        def save_b() -> None:
            started = time.time()
            try:
                holder["save"] = self.call("u: B's pool save (holds the Agent's pool lock during the slow reload)", "POST",
                                           f"/api/v1/domains/{b_id}/php/pool",
                                           {"version": version, "pool_config": {"pm": "dynamic", "pm_max_children": 7, "pm_start_servers": 2,
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
        shown = self.sc("u page of A while B's pool save holds the lock", a_id)
        read_seconds = round(time.time() - started, 1)
        rows = self.domains_rows("u while the lock is held")
        thread.join(timeout=300)
        window = self.window12("u", since - 1)
        self.lab12("u-slow-reload-removed", "lab-slow-reload", domain=DOMAIN_A, remove=True)
        again = self.sc("u page of A after the lock was released", a_id)
        rows_after = self.domains_rows("u after")
        body = shown["body"]
        pool = file_of(body, "php_pool")
        panel = window["by_unit"].get("celikpanel-panel.service") or []
        save = holder.get("save") or {}
        checks = [
            ("lab: PHP-FPM's reload waits 75 s (drop-in active)", "sleep" in str(slow.get("exec_reload")), slow.get("exec_reload")),
            ("B's pool save was in progress across the read (answered after it)", bool(save) and holder.get("seconds", 0) >= read_seconds,
             {"save_status": save.get("status"), "save_seconds": holder.get("seconds"), "read_seconds": read_seconds,
              "error": holder.get("error")}),
            ("GET site-config answers 200 with the vhost read and the pool unknown, reason not_read, action recheck",
             shown["status"] == 200 and file_of(body, "nginx_vhost").get("state") == "managed_unchanged"
             and pool.get("state") == "unknown" and pool.get("reason") == "not_read" and pool.get("actions") == ["recheck"],
             self.page_brief(body) | {"status": shown["status"], "read_seconds": read_seconds}),
            ("overall_state unknown and overall_kind php_pool (the unread file outranks CelikPanel's unchanged text)",
             body.get("overall_state") == "unknown" and body.get("overall_kind") == "php_pool",
             {"overall_state": body.get("overall_state"), "overall_kind": body.get("overall_kind")}),
            ("the Panel's journal names the pool it could not read",
             any(DOMAIN_A in l and "php_pool" in l and "could not read" in l for l in panel),
             [l for l in panel if DOMAIN_A in l][:6]),
            ("the Domains row says a configuration file could not be read (state unknown, kind php_pool)",
             (rows.get(DOMAIN_A) or {}).get("state") == "unknown" and (rows.get(DOMAIN_A) or {}).get("kind") == "php_pool",
             {"row_during": rows.get(DOMAIN_A), "row_after": rows_after.get(DOMAIN_A),
              "documented_limit": "docs: the Domains list's site_config is built from the ledger rows only; a not_read leaves no row"}),
            ("after the lock is released the page reads the pool again (managed_unchanged)",
             file_of(again["body"], "php_pool").get("state") == "managed_unchanged", self.page_brief(again["body"]))]
        self.judge("U", checks, {"journal": window["_file"]})

    # -- RN, RP1, RP2 ---------------------------------------------------------------------------------------------

    def rn_nginx(self) -> None:
        a_id = self.a_id()
        self.restore10("rn", DOMAIN_A)
        self.owner10("rn-replace", "owner-vhost10", domain=DOMAIN_A, action="replace", reload=True)
        put = self.owner10("rn-include", "owner-include", domain=DOMAIN_A, name="set10-dup-root.conf", kind="duplicate-root", reload=True)
        before = self.read10("rn-1-before")
        shown = self.sc("rn shown", a_id)
        since = self.guest_epoch()
        answer = self.take_post("rn take while an owner's include-directory file conflicts with CelikPanel's text",
                                a_id, shown["body"], None)
        time.sleep(s10.SETTLE)
        after = self.read10("rn-2-after", dump=True)
        http = self.http10("rn-2-after", DOMAIN_A, "/")
        window = self.window12("rn", since - 1)
        b, a = before["sites"][DOMAIN_A], after["sites"][DOMAIN_A]
        page = self.sc("rn page after the refusal", a_id)
        self.judge("RN", [
            ("the owner's conflicting file passed nginx -t with the owner's vhost", (put.get("nginx_test") or {}).get("returncode") == 0,
             put.get("nginx_test")),
            ("take answers 502 SITE_CONFIG_NGINX_REFUSED with restore confirmed",
             answer["status"] == 502 and answer["body"].get("code") == "SITE_CONFIG_NGINX_REFUSED"
             and answer["body"].get("restore") == "confirmed", answer["entry"]),
            ("the previous file is in place (sha256, mode, owner, group)",
             (a["vhost"].get("sha256"), a["vhost"].get("mode"), a["vhost"].get("uid"), a["vhost"].get("gid"))
             == (b["vhost"].get("sha256"), b["vhost"].get("mode"), b["vhost"].get("uid"), b["vhost"].get("gid")),
             {k: [b["vhost"].get(k), a["vhost"].get(k)] for k in ("sha256", "inode", "mode", "uid", "gid")}),
            ("nginx passes -t and serves the owner's file", (after.get("nginx_test") or {}).get("returncode") == 0
             and "owner-replaced" in str(http.get("body") or ""), {"nginx_test": after.get("nginx_test"), "body": str(http.get("body") or "")[:80]})],
            {"backups_before": [x.get("path") for x in b["side_files"]["backups"]],
             "backups_after": [x.get("path") for x in a["side_files"]["backups"]],
             "page_after": self.page_brief(page["body"]), "journal": window["_file"]})
        self.owner10("rn-include-back", "owner-include", domain=DOMAIN_A, name="set10-dup-root.conf", remove=True, reload=False)
        self.restore10("rn-end", DOMAIN_A)
        self.owner10("rn-reload", "owner-include", domain=DOMAIN_A, name="set10-x.conf", remove=True, reload=True)

    def pool_refusal(self, name: str, how: str) -> None:
        a_id = self.a_id()
        self.restore_pool(name)
        self.owner12(f"{name}-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
        shown = self.sc(f"{name} shown", a_id)
        pool = file_of(shown["body"], "php_pool")
        before = self.pool12(f"{name}-0-before", DOMAIN_A)
        _, p0 = self.the_pool(before)
        if how == "broken":
            cause = self.owner12(f"{name}-broken-pool-file", "owner-broken-pool", domain=DOMAIN_A)
            cause_ok = (cause.get("php_fpm_test") or {}).get("returncode") not in (0, None)
        else:
            cause = self.owner12(f"{name}-php-stop", "owner-php12", domain=DOMAIN_A, action="stop")
            cause_ok = (cause.get("after") or {}).get("ActiveState") in ("inactive", "failed")
        since = self.guest_epoch()
        answer = self.take_post(f"{name} take on the owner's pool", a_id, pool, "php_pool")
        time.sleep(s10.SETTLE)
        after = self.pool12(f"{name}-1-after", DOMAIN_A)
        _, p1 = self.the_pool(after)
        window = self.window12(name, since - 1)
        unit = after.get("php_unit") or ""
        abody = answer["body"]
        restore = abody.get("restore")
        detail = abody.get("detail")
        error = str(abody.get("error") or "")
        checks = [
            ("the cause is in place (" + ("php-fpm -t fails because of the owner's pool file" if how == "broken"
                                          else "PHP-FPM stopped by the owner") + ")", cause_ok,
             {k: cause.get(k) for k in ("php_fpm_test", "after", "systemctl")}),
            ("take answers 502 SITE_CONFIG_SERVICE_REFUSED, reason php_pool, with a restore fact",
             answer["status"] == 502 and abody.get("code") == "SITE_CONFIG_SERVICE_REFUSED" and abody.get("reason") == "php_pool"
             and restore in ("confirmed", "unconfirmed"), answer["entry"]),
            ("the owner's pool is in place again (sha256, mode, owner, group)",
             (p1.get("sha256"), p1.get("mode"), p1.get("uid"), p1.get("gid")) == (p0.get("sha256"), p0.get("mode"), p0.get("uid"), p0.get("gid")),
             {k: [p0.get(k), p1.get(k)] for k in ("sha256", "inode", "mode", "uid", "gid")}),
        ]
        if how == "broken":
            checks.append(("detail config_refused; restore confirmed (the put-back and the reload after it held)",
                           detail == "config_refused" and restore == "confirmed", {"detail": detail, "restore": restore}))
        else:
            checks.append(("detail reload_failed; restore unconfirmed and the sentence says the administrator checks the file",
                           detail == "reload_failed" and restore == "unconfirmed" and "not confirmed" in error,
                           {"detail": detail, "restore": restore, "error": error, "details": abody.get("details")}))
        self.judge(name.upper(), checks, {"php_unit_after": after.get("php_unit_state"),
                                          "php_lines": (window["by_unit"].get(unit + ".service") or [])[-12:],
                                          "backups_after": [b.get("path") for b in (p1.get("side_files") or {}).get("backups") or []],
                                          "pool_dir_after": [e.get("path") for e in (after.get("pool_dir") or {}).get("entries") or []],
                                          "journal": window["_file"]})
        if how == "broken":
            self.owner12(f"{name}-broken-pool-file-removed", "owner-broken-pool", domain=DOMAIN_A, remove=True)
            self.owner12(f"{name}-php-restart", "owner-php12", domain=DOMAIN_A, action="restart")
        else:
            self.owner12(f"{name}-php-start", "owner-php12", domain=DOMAIN_A, action="start")
        self.restore_pool(f"{name}-end")

    # -- P (apply) and B4 -----------------------------------------------------------------------------------------

    def p_apply(self) -> None:
        a_id = self.a_id()
        refs = self.s10["ref"]
        self.restore10("p", DOMAIN_A)
        self.panel10("p", "stop")
        self.snap10("p-lab-stale", "lab-stale-managed", domains=[DOMAIN_A])
        self.owner11("p-chattr-plus-i", "owner-chattr", domain=DOMAIN_A, flag="+i")
        fired = self.panel10("p", "start")
        time.sleep(s10.SETTLE)
        locked = self.sc("p page after the start (locked)", a_id)
        self.owner11("p-chattr-minus-i", "owner-chattr", domain=DOMAIN_A, flag="-i")
        before = self.read10("p-1-unlocked", domains=[DOMAIN_A])
        recheck = self.sc("p check again after the unlock", a_id)
        rows = self.domains_rows("p after the unlock")
        ledger = [r for r in self.ledger_rows("p-1", a_id) if r.get("kind") == "nginx_vhost"]
        body = recheck["body"]
        since = self.guest_epoch()
        applied = self.post_raw("p apply (Write CelikPanel's text now)", f"/api/v1/domains/{a_id}/site-config/apply",
                                {"file_sha256": body.get("file_sha256"), "render_sha256": body.get("render_sha256")},
                                secrets.token_hex(16))
        time.sleep(s10.SETTLE)
        after = self.read10("p-2-after-apply", domains=[DOMAIN_A])
        window = self.window12("p-apply", since - 1)
        page = self.sc("p page after apply", a_id)
        rows2 = self.domains_rows("p after apply")
        b, a = before["sites"][DOMAIN_A], after["sites"][DOMAIN_A]
        nginx = window["by_unit"].get("nginx.service") or []
        self.judge("P-apply", [
            ("the Panel started with A locked: A kept, page unwritable (write_refused, immutable)",
             bool(fired.get("ok")) and locked["body"].get("state") == "unwritable" and locked["body"].get("protection") == "immutable",
             {k: locked["body"].get(k) for k in ("state", "reason", "system_error", "protection", "actions")}),
            ("after the unlock the read answers managed_unchanged, reason protection_removed, action apply",
             body.get("state") == "managed_unchanged" and body.get("reason") == "protection_removed" and body.get("actions") == ["apply"],
             {k: body.get(k) for k in ("state", "reason", "protection", "system_error", "actions", "file_state")}),
            ("the ledger row keeps the reason protection_removed", bool(ledger) and ledger[-1].get("state_reason") == "protection_removed",
             ledger[-1:] if ledger else None),
            ("the Domains row agrees (managed_unchanged)", (rows.get(DOMAIN_A) or {}).get("state") in ("managed_unchanged", None), rows.get(DOMAIN_A)),
            ("POST apply answers 200 (outcome written)", applied["status"] == 200 and applied["body"].get("outcome") in ("written", "taken"),
             applied["entry"]),
            ("A is written: new inode, CelikPanel's text, valid header; no dated copy",
             a["vhost"].get("inode") != b["vhost"].get("inode") and a["vhost"].get("sha256") == refs[DOMAIN_A]["sha256"]
             and bool((a["vhost"].get("header") or {}).get("valid")) and len(a["side_files"]["backups"]) == len(b["side_files"]["backups"]),
             {"inode": [b["vhost"].get("inode"), a["vhost"].get("inode")], "sha256_is_panel_text": a["vhost"].get("sha256") == refs[DOMAIN_A]["sha256"],
              "backups": [len(b["side_files"]["backups"]), len(a["side_files"]["backups"])]}),
            ("nginx was reloaded after apply", bool(s10.lines_after(nginx, since, "Reloaded")), s10.lines_after(nginx, since, "Reloaded")),
            ("the page and the row then say managed_unchanged with no apply action",
             page["body"].get("state") == "managed_unchanged" and "apply" not in (page["body"].get("actions") or [])
             and (rows2.get(DOMAIN_A) or {}).get("state") in ("managed_unchanged", None),
             {"page": {k: page["body"].get(k) for k in ("state", "reason", "actions")}, "row": rows2.get(DOMAIN_A)})],
            {"journal": window["_file"]})
        # B4 (set11): take refused on an owner-edited file locked after the read.
        self.restore10("b4", DOMAIN_A)
        self.owner10("b4-edit", "owner-vhost10", domain=DOMAIN_A, action="append-location", reload=True)
        shown = self.sc("b4 shown (owner_edited)", a_id)
        self.owner11("b4-chattr-plus-i", "owner-chattr", domain=DOMAIN_A, flag="+i")
        st0 = self.stat_a("b4-0")
        taken = self.take_post("b4 take while the file is locked", a_id, shown["body"], None)
        time.sleep(s10.SETTLE)
        st1 = self.stat_a("b4-1")
        page4 = self.sc("b4 page after the refused take", a_id)
        rows4 = self.domains_rows("b4 after the refused take")
        ledger4 = [r for r in self.ledger_rows("b4-1", a_id) if r.get("kind") == "nginx_vhost"]
        self.owner11("b4-chattr-minus-i", "owner-chattr", domain=DOMAIN_A, flag="-i")
        p4 = page4["body"]
        self.judge("B4", [
            ("take answers 409 SITE_CONFIG_UNWRITABLE, reason write_refused",
             taken["status"] == 409 and taken["body"].get("code") == "SITE_CONFIG_UNWRITABLE" and taken["body"].get("reason") == "write_refused",
             taken["entry"]),
            ("no dated copy was left and the owner's file is untouched (sha256, inode)",
             len(st1["side"]["backups"]) == len(st0["side"]["backups"])
             and (st0["vhost"].get("sha256"), st0["vhost"].get("inode")) == (st1["vhost"].get("sha256"), st1["vhost"].get("inode")),
             {"backups": [[x.get("path") for x in st0["side"]["backups"]], [x.get("path") for x in st1["side"]["backups"]]],
              "inode": [st0["vhost"].get("inode"), st1["vhost"].get("inode")]}),
            ("the page shows both facts: unwritable with file_state owner_edited, EPERM, immutable",
             p4.get("state") == "unwritable" and p4.get("file_state") == "owner_edited" and p4.get("protection") == "immutable",
             {k: p4.get(k) for k in ("state", "file_state", "reason", "system_error", "protection", "actions")}),
            ("the Domains row agrees with the page (unwritable)", (rows4.get(DOMAIN_A) or {}).get("state") == "unwritable",
             {"row": rows4.get(DOMAIN_A), "ledger": [{k: r.get(k) for k in ("state", "state_reason", "backup_path")} for r in ledger4[-1:]]})])
        self.restore10("b4-end", DOMAIN_A)

    # -- A (probe addresses) --------------------------------------------------------------------------------------

    def probe_case(self, label: str, edit_mode: str, edit_args: dict, expect_dst: str | None) -> dict:
        self.restore10(label, DOMAIN_A)
        self.owner10(f"{label}-edit", "owner-vhost10", domain=DOMAIN_A, action="append-location", reload=True)
        edit = self.owner12(f"{label}-bind", "owner-vhost12", domain=DOMAIN_A, action=edit_mode, reload=True, **edit_args)
        before = self.stat_a(f"{label}-0")
        obs = self.watched(label, lambda: self.certificate_post(label))
        after = self.stat_a(f"{label}-1")
        answer, body = obs["result"], obs["result"]["body"]
        dsts = syn_destinations(obs["net"])
        gets = [g | {"dst": dsts.get(next((r.get("sport") for r in obs["net"] if r.get("request") == g["request"] and r.get("t") == g["t"]), None))}
                for g in obs["gets"] if PROBE_PATH + "celikpanel-probe-" in g["request"]]
        last = max((g.get("answer_t") or g["t"]) for g in gets) if gets else None
        to_ca = s11.syn_rows(obs["net"], 443, last)
        panel = self.journal_lines(obs["window"], "celikpanel-panel.service")
        return {"edit": edit, "before": before, "after": after, "answer": answer, "body": body, "gets": gets,
                "to_ca": to_ca, "obs": obs, "panel": panel, "probe": s11.summary_of(obs["probe_files"], obs["gets"])}

    def a_addresses(self) -> None:
        addresses = self.snap12("a-addresses", "read-addresses12")
        own = (addresses.get("ipv4") or [None])[0]
        results, checks = {}, []
        # 1. the guest's own address
        if own:
            r = self.probe_case("a-own", "bind-address", {"address": own}, own)
            names_ok = [g.get("host") for g in r["gets"]] == [DOMAIN_A, "www." + DOMAIN_A]
            checks += [
                (f"own address: the owner's block listens on {own}:80 only, nginx -t passed",
                 (r["edit"].get("nginx_test") or {}).get("returncode") == 0, r["edit"].get("changed")),
                (f"own address: the probe asked {own} under each name and nginx served the probe's bytes (ready)",
                 names_ok and all(g.get("dst") == own for g in r["gets"])
                 and all(s11.answered_with(next(iter(r["obs"]["probe_files"].values()), {}), g) for g in r["gets"]),
                 [{k: g.get(k) for k in ("host", "dst", "status", "answer")} for g in r["gets"]]),
                ("own address: then the request went to the pinned authority (a SYN to 443) and the answer is "
                 "502 CERTIFICATE_ISSUE_FAILED authority_unreachable",
                 bool(r["to_ca"]) and r["answer"]["status"] == 502 and r["body"].get("reason") == "authority_unreachable",
                 {"answer": {k: r["body"].get(k) for k in ("code", "reason")} | {"status": r["answer"]["status"]}, "syn_443": r["to_ca"][:3]}),
                self.file_untouched(r["before"], r["after"])]
            results["own"] = {k: r[k] for k in ("answer", "gets", "probe")} | {"files": r["obs"]["files"]}
        else:
            checks.append(("own address: the guest has a global IPv4 address", None, addresses))
        # 2. [::]:80 only
        r = self.probe_case("a-v6", "ipv6-only", {}, "::1")
        checks += [
            ("[::]:80 only: nginx -t passed and A's block no longer listens on IPv4", (r["edit"].get("nginx_test") or {}).get("returncode") == 0,
             {"changed": r["edit"].get("changed"), "listening_80": r["edit"].get("listening_80")}),
            ("[::]:80 only: the probe asked ::1 under each name and nginx served the probe (ready)",
             [g.get("host") for g in r["gets"]] == [DOMAIN_A, "www." + DOMAIN_A] and all(g.get("dst") == "::1" for g in r["gets"])
             and all(s11.answered_with(next(iter(r["obs"]["probe_files"].values()), {}), g) for g in r["gets"]),
             [{k: g.get(k) for k in ("host", "dst", "status", "answer")} for g in r["gets"]]),
            ("[::]:80 only: the request went ahead (SYN to 443); 502 authority_unreachable",
             bool(r["to_ca"]) and r["answer"]["status"] == 502 and r["body"].get("reason") == "authority_unreachable",
             {"answer": {k: r["body"].get(k) for k in ("code", "reason")} | {"status": r["answer"]["status"]}, "syn_443": r["to_ca"][:3]})]
        results["v6"] = {k: r[k] for k in ("answer", "gets", "probe")} | {"files": r["obs"]["files"]}
        # 3. an address this guest does not have
        if NONLOCAL_ADDRESS in (addresses.get("ipv4") or []):
            raise base.StepFailed(f"{NONLOCAL_ADDRESS} is an address of this guest")
        self.lab12("a-nonlocal-bind-on", "lab-nonlocal-bind", value=1)
        r = self.probe_case("a-nonlocal", "bind-address", {"address": NONLOCAL_ADDRESS}, None)
        notrun = [l for l in r["panel"] if "validation_probe=not_run" in l]
        checks += [
            (f"not this server's address: the owner's block listens on {NONLOCAL_ADDRESS}:80 (lab: ip_nonlocal_bind=1), nginx -t passed",
             (r["edit"].get("nginx_test") or {}).get("returncode") == 0, {"changed": r["edit"].get("changed"), "listening_80": r["edit"].get("listening_80")}),
            ("not this server's address: no probe request was sent; the request went ahead (SYN to 443)",
             not r["gets"] and bool(s11.syn_rows(r["obs"]["net"], 443)), {"gets": r["gets"], "syn_443": s11.syn_rows(r["obs"]["net"], 443)[:3]}),
            ("not this server's address: the Panel's journal says validation_probe=not_run validation_unknown_reason=address_not_local "
             f"validation_address={NONLOCAL_ADDRESS}",
             any("address_not_local" in l and NONLOCAL_ADDRESS in l for l in notrun), notrun),
            ("not this server's address: the answer carries validation_probe not_run, validation_unknown_reason, the address and the sentence",
             r["body"].get("validation_probe") == "not_run" and r["body"].get("validation_unknown_reason") == "address_not_local"
             and r["body"].get("validation_address") == NONLOCAL_ADDRESS and bool(r["body"].get("validation_note")),
             {"status": r["answer"]["status"], "body": {k: r["body"].get(k) for k in ("code", "reason", "validation_probe",
                                                                                       "validation_unknown_reason", "validation_address",
                                                                                       "validation_note", "status")},
              "why_this_may_not_hold": "the authority is pinned to the guest's loopback, so the request fails; the fields are "
                                       "documented on the success answer"})]
        results["nonlocal"] = {k: r[k] for k in ("answer", "gets", "probe")} | {"files": r["obs"]["files"], "journal_lines": notrun}
        # 3b. the page's sentence: a certificate_validation reason first (the owner's file without the managed include),
        #     then the owner binds to the non-local address with the include back; the page's read probes.
        self.restore10("a-page", DOMAIN_A)
        self.owner11("a-page-remove-include", "owner-vhost11", domain=DOMAIN_A, action="remove-managed-include", reload=True)
        refused = self.watched("a-page", lambda: self.certificate_post("a-page: a refused request sets the reason"))
        results["page_first_request"] = {"probe": s11.summary_of(refused["probe_files"], refused["gets"]), "files": refused["files"]}
        refused = refused["result"]
        self.owner11("a-page-include-back", "owner-vhost11", domain=DOMAIN_A, action="add-managed-include", reload=True)
        self.owner12("a-page-bind", "owner-vhost12", domain=DOMAIN_A, action="bind-address", address=NONLOCAL_ADDRESS, reload=True)
        page = self.sc("a-page: the Configuration file page reads", self.a_id())
        ledger = [x for x in self.ledger_rows("a-page", self.a_id()) if x.get("kind") == "nginx_vhost"]
        pb = page["body"]
        checks.append(("the page (reason certificate_validation) reads validation unknown, address_not_local, the address; the reason stays",
                       pb.get("validation") == "unknown" and pb.get("validation_unknown_reason") == "address_not_local"
                       and pb.get("validation_address") == NONLOCAL_ADDRESS and bool(ledger) and ledger[-1].get("state_reason") == "certificate_validation",
                       {"refused_first": {k: refused["body"].get(k) for k in ("code", "reason", "detail")} | {"status": refused["status"]},
                        "page": {k: pb.get(k) for k in ("state", "validation", "validation_unknown_reason", "validation_address",
                                                         "pending_reason", "resolved_reason")},
                        "ledger_reason": ledger[-1].get("state_reason") if ledger else None}))
        results["page"] = {k: pb.get(k) for k in ("state", "validation", "validation_unknown_reason", "validation_address", "pending_reason")}
        self.restore10("a-page-end", DOMAIN_A)
        self.lab12("a-nonlocal-bind-off", "lab-nonlocal-bind", value=int(addresses.get("ip_nonlocal_bind") or 0))
        # 4. nginx stopped
        self.restore10("a-stop", DOMAIN_A)
        self.owner10("a-stop-edit", "owner-vhost10", domain=DOMAIN_A, action="append-location", reload=True)
        stopped = self.owner11("a-stop-nginx", "owner-nginx", action="stop")
        try:
            answer = self.certificate_post("a-stop: nginx stopped")
        finally:
            started = self.owner11("a-stop-nginx-start", "owner-nginx", action="start")
        checks.append(("nginx stopped: 503 CERTIFICATE_VALIDATION_UNKNOWN, detail unknown (unchanged)",
                       stopped.get("is_active") in ("inactive", "failed") and answer["status"] == 503
                       and answer["body"].get("code") == "CERTIFICATE_VALIDATION_UNKNOWN" and answer["body"].get("detail") == "unknown",
                       {"status": answer["status"], "body": {k: answer["body"].get(k) for k in ("code", "detail")},
                        "nginx_after": started.get("is_active")}))
        self.restore10("a-end", DOMAIN_A)
        self.site_config("a cleanup read", self.a_id())
        self.judge("A", checks, results)

    # -- N (application unit) -------------------------------------------------------------------------------------

    def install_node(self) -> dict:
        listed = self.call("n: Node.js versions (GET /api/v1/runtimes/node)", "GET", "/api/v1/runtimes/node")
        installed = (listed["_parsed"] or {}).get("installed") if isinstance(listed["_parsed"], dict) else None
        if installed:
            return {"version": installed[0], "how": "already installed"}
        lts = self.call("n: the LTS list (GET /api/v1/runtimes/node/lts)", "GET", "/api/v1/runtimes/node/lts", timeout=60)
        version = None
        parsed = lts["_parsed"]
        rows = parsed if isinstance(parsed, list) else (parsed or {}).get("versions") or (parsed or {}).get("lts") or [] if isinstance(parsed, dict) else []
        for row in rows:
            candidate = row.get("version") if isinstance(row, dict) else row
            if isinstance(candidate, str) and re.fullmatch(r"v?\d+\.\d+\.\d+", candidate):
                version = candidate.lstrip("v")
                break
        version = version or NODE_FALLBACK
        posted = self.call(f"n: install Node.js {version} through the Panel", "POST", "/api/v1/runtimes/node",
                           {"version": version, "request_id": secrets.token_hex(16)}, timeout=120)
        op = ((posted["_parsed"] or {}).get("operation") or {}) if isinstance(posted["_parsed"], dict) else {}
        status = op.get("status")
        deadline = time.time() + 900
        while op.get("id") and status not in ("succeeded", "failed", "canceled", "cancelled") and time.time() < deadline:
            time.sleep(10)
            polled = self.api("GET", f"/api/v1/service/operation?id={op['id']}", purpose="n: the install's state", timeout=30)
            got = polled.json() if polled.status == 200 else {}
            op = (got.get("operation") if isinstance(got, dict) and "operation" in got else got) or op
            status = op.get("status")
        self.current["node_install"] = {"version": version, "post_status": posted["status"], "final": op}
        listed = self.call("n: Node.js versions after the install", "GET", "/api/v1/runtimes/node")
        installed = (listed["_parsed"] or {}).get("installed") if isinstance(listed["_parsed"], dict) else None
        return {"version": version if installed and version in installed else None, "how": "installed through the Panel",
                "status": status, "installed": installed}

    def hosting_node(self, label: str, domain_id: int, version: str, marker: str) -> dict:
        # The runtime's own binary by its path: the unit runs the command through a login shell (/bin/sh -lc), whose
        # profile resets PATH on Debian, so a bare `node` exits 127 there (set12 Debian run a).
        command = "/opt/celikpanel/runtimes/node/%s/bin/node -e \"require('http').createServer((q,s)=>s.end('%s')).listen(process.env.PORT)\"" % (version, marker)
        entry = self.call(label, "PUT", f"/api/v1/domains/{domain_id}/hosting",
                          {"project_type": "node", "start_command": command, "runtime_version": version}, timeout=600)
        return {"status": entry["status"], "body": entry["_parsed"] if isinstance(entry["_parsed"], dict) else {}}

    def n_unit(self) -> None:
        node = self.install_node()
        version = node.get("version")
        if not version:
            self.judge("N", [("the unit cells need a Node.js version installed through the Panel", None, node)])
            return
        n_id = self.create_php(DOMAIN_N)
        first = self.hosting_node("n: N becomes a Node site", n_id, version, "set12-n1")
        u0 = self.snap12("n-0-unit", "read-unit12", domain=DOMAIN_N)
        unit0 = u0.get("unit") or {}
        self.keep10("n-0-unit", unit0)
        checks = [("the unit is written with the \"# celikpanel-render v2\" header and its digest holds; the app runs",
                   first["status"] == 200 and bool((unit0.get("header") or {}).get("valid"))
                   and (u0.get("state") or {}).get("ActiveState") == "active",
                   {"status": first["status"], "header": unit0.get("header"), "state": u0.get("state")})]
        self.owner10("n-dropin", "owner-unit10", domain=DOMAIN_N, action="add-dropin")
        d0 = self.snap12("n-1-dropin", "read-unit12", domain=DOMAIN_N)
        own0 = next((e for e in (d0.get("dropin_dir") or {}).get("entries") or [] if str(e.get("path", "")).endswith("set10-owner.conf")), {})
        second = self.hosting_node("n: a hosting change (another start command)", n_id, version, "set12-n2")
        u2 = self.snap12("n-2-after-a-change", "read-unit12", domain=DOMAIN_N)
        unit2 = u2.get("unit") or {}
        own2 = next((e for e in (u2.get("dropin_dir") or {}).get("entries") or [] if str(e.get("path", "")).endswith("set10-owner.conf")), {})
        checks.append(("a hosting change rewrites CelikPanel's unit (sealed) and leaves the owner's drop-in untouched; systemd applies it",
                       second["status"] == 200 and "set12-n2" in str(unit2.get("text")) and bool((unit2.get("header") or {}).get("valid"))
                       and (own2.get("sha256"), own2.get("inode")) == (own0.get("sha256"), own0.get("inode"))
                       and "SET10_OWNER=dropin" in str(u2.get("environment")),
                       {"status": second["status"], "dropin": [[own0.get("sha256"), own0.get("inode")], [own2.get("sha256"), own2.get("inode")]],
                        "environment": u2.get("environment")}))
        edit = self.owner10("n-unit-line", "owner-unit10", domain=DOMAIN_N, action="append-directive")
        edited = edit.get("after") or {}
        third = self.hosting_node("n: a hosting change on the owner-edited unit", n_id, version, "set12-n3")
        u3 = self.snap12("n-3-after-the-refused-change", "read-unit12", domain=DOMAIN_N)
        unit3 = u3.get("unit") or {}
        checks.append(("the change is refused 409 SITE_CONFIG_OWNER_EDITED (reason app_unit); the unit's bytes and inode stay",
                       third["status"] == 409 and third["body"].get("code") == "SITE_CONFIG_OWNER_EDITED" and third["body"].get("reason") == "app_unit"
                       and (unit3.get("sha256"), unit3.get("inode")) == (edited.get("sha256"), edited.get("inode")),
                       {"answer": {k: third["body"].get(k) for k in ("code", "reason", "detail")} | {"status": third["status"]},
                        "sha256": [edited.get("sha256"), unit3.get("sha256")]}))
        shown = self.sc("n page", n_id)
        unitfile = file_of(shown["body"], "app_unit")
        checks.append(("the page lists the unit owner_edited with the drop-in directory as its include_dir",
                       unitfile.get("state") == "owner_edited" and str(unitfile.get("include_dir") or "").endswith(".service.d"),
                       {k: unitfile.get(k) for k in ("state", "include_dir", "actions", "path")}))
        since = self.guest_epoch()
        taken = self.take_post("n take CelikPanel's unit (kind app_unit)", n_id, unitfile, "app_unit")
        time.sleep(s10.SETTLE)
        u4 = self.snap12("n-4-after-take", "read-unit12", domain=DOMAIN_N)
        unit4 = u4.get("unit") or {}
        own4 = next((e for e in (u4.get("dropin_dir") or {}).get("entries") or [] if str(e.get("path", "")).endswith("set10-owner.conf")), {})
        window = self.window12("n-take", since - 1)
        systemd = [l for l in window.get("systemd_all") or [] if (s10.journal_epoch(l) or 0) >= since]
        checks += [
            ("take answers 200 outcome taken; a dated copy holds the owner's unit; the unit is CelikPanel's sealed text",
             taken["status"] == 200 and taken["body"].get("outcome") == "taken"
             and any(b.get("sha256") == edited.get("sha256") for b in (unit4.get("side_files") or {}).get("backups") or [])
             and bool((unit4.get("header") or {}).get("valid")) and "SET10_OWNER=unit" not in str(unit4.get("text")),
             {"answer": taken["entry"], "backups": [b.get("path") for b in (unit4.get("side_files") or {}).get("backups") or []]}),
            ("the owner's drop-in is untouched after the take", (own4.get("sha256"), own4.get("inode")) == (own0.get("sha256"), own0.get("inode")),
             [own0.get("sha256"), own4.get("sha256")]),
            ("systemd was reloaded and the running app restarted (try-restart: MainPID changed, active)",
             any("Reload" in l for l in systemd) and (u4.get("state") or {}).get("MainPID") != (u3.get("state") or {}).get("MainPID")
             and (u4.get("state") or {}).get("ActiveState") == "active",
             {"systemd": systemd[:12], "main_pid": [(u3.get("state") or {}).get("MainPID"), (u4.get("state") or {}).get("MainPID")]})]
        self.judge("N", checks, {"node": node, "journal": window["_file"]})

    # -- L (pool deletion) ----------------------------------------------------------------------------------------

    def l_delete(self) -> None:
        c_id = self.create_php(DOMAIN_C)
        self.owner12("l-pool-edit", "owner-pool12", domain=DOMAIN_C, action="append", reload=True)
        before = self.pool12("l-0", DOMAIN_C)
        path, p0 = self.the_pool(before)
        gone = self.call(f"l: delete {DOMAIN_C} (Domains page)", "DELETE", f"/api/v1/domains/{c_id}", timeout=600)
        time.sleep(s10.SETTLE)
        after = self.snap12("l-1-paths", "read-paths12", paths=[path] if path else [])
        p1 = (after.get("files") or {}).get(path) or {}
        backups = (p1.get("side_files") or {}).get("backups") or []
        for index, b in enumerate(backups):
            self.keep10(f"l-pool-backup-{index}", b)
        self.judge("L", [
            ("the deletion answers 200", gone["status"] == 200, gone.get("answer") or gone["status"]),
            ("the pool is removed and a dated copy with the owner's bytes is kept beside it",
             not p1.get("exists") and any(b.get("sha256") == p0.get("sha256") for b in backups),
             {"pool_after": p1.get("exists"), "backups": [{k: b.get(k) for k in ("path", "sha256")} for b in backups],
              "owner_sha256": p0.get("sha256")})])

    # -- browser (SET12_HAND; set11's handover with named preparations) -------------------------------------------

    def prep(self, name: str) -> dict:
        a_id = self.a_id()
        if name == "pool-edited":
            self.restore10("prep-pool", DOMAIN_A)
            self.restore_pool("prep-pool")
            self.owner12("prep-pool-edit", "owner-pool12", domain=DOMAIN_A, action="append", reload=True)
            body = self.sc("prep pool-edited read", a_id)["body"]
            return {"pool": file_of(body, "php_pool").get("state"), "overall": body.get("overall_state")}
        if name == "php-stop":
            return {"php": (self.owner12("prep-php-stop", "owner-php12", domain=DOMAIN_A, action="stop").get("after") or {}).get("ActiveState")}
        if name == "php-start":
            return {"php": (self.owner12("prep-php-start", "owner-php12", domain=DOMAIN_A, action="start").get("after") or {}).get("ActiveState")}
        if name == "broken-add":
            return {"test": (self.owner12("prep-broken-add", "owner-broken-pool", domain=DOMAIN_A).get("php_fpm_test") or {}).get("returncode")}
        if name == "broken-remove":
            self.owner12("prep-broken-remove", "owner-broken-pool", domain=DOMAIN_A, remove=True)
            return {"php": (self.owner12("prep-php-restart", "owner-php12", domain=DOMAIN_A, action="restart").get("after") or {}).get("ActiveState")}
        if name == "apply-state":
            self.restore_pool("prep-apply")
            self.restore10("prep-apply", DOMAIN_A)
            self.panel10("prep-apply", "stop")
            self.snap10("prep-apply-lab-stale", "lab-stale-managed", domains=[DOMAIN_A])
            self.owner11("prep-apply-chattr-plus-i", "owner-chattr", domain=DOMAIN_A, flag="+i")
            self.panel10("prep-apply", "start")
            time.sleep(s10.SETTLE)
            self.sc("prep apply-state locked read", a_id)
            self.owner11("prep-apply-chattr-minus-i", "owner-chattr", domain=DOMAIN_A, flag="-i")
            return {"note": "the page's next read is the check again"}
        if name == "end":
            self.restore10("prep-end", DOMAIN_A)
            self.restore_pool("prep-end")
            return {"ok": True}
        return {"unknown": name}

    def browser_window(self, name: str, wait_seconds: int, extra: dict) -> dict:
        prefix = os.environ.get("SET12_HAND")
        if not prefix:
            return {"window": name, "skipped": "no browser in this lab (SET12_HAND is not set)"}
        hand = s11.HAND_ROOT / f"{prefix}-{name}"
        if hand.exists():
            return {"window": name, "skipped": f"the handover directory {hand} exists; a window is never reused"}
        hand.mkdir(parents=True, mode=0o755)
        s11.SECRET_ROOT.mkdir(parents=True, mode=0o700, exist_ok=True)
        os.chmod(s11.SECRET_ROOT, 0o700)
        secret = s11.SECRET_ROOT / f"{prefix}-{name}.json"
        leaf = self.refresh_pin()
        handover = {"schema": "celikpanel/set12-handover/v1", "window": name, "local_port": self.local_port,
                    "base": f"https://127.0.0.1:{self.local_port}", "username": self.state.get("username"),
                    "tls_leaf_sha256": leaf, "at": base.utc_now(), "password_file": str(secret)} | extra
        descriptor = os.open(secret, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as handle:
            json.dump({"username": self.state.get("username"), "password": self._password,
                       "redact": [base.ACCEPTANCE_FIXTURE_KEY]}, handle)
        (hand / "ready.json").write_text(json.dumps(handover, indent=2, sort_keys=True) + "\n")
        print(json.dumps({"set12": "ready", "window": name, "hand": str(hand), "port": self.local_port}), flush=True)
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
                for request in sorted(hand.glob("prep-request-*")):
                    key = request.name[len("prep-request-"):]
                    marker = hand / f"prep-done-{key}.json"
                    if marker.exists():
                        continue
                    what = key.rsplit("-", 1)[0]
                    try:
                        result = self.prep(what)
                    except Exception as exc:  # noqa: BLE001 - written for the browser and kept
                        result = {"error": self.error_text(exc)}
                    record = {"prep": what, "at": base.utc_now(), "result": result}
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

    def final_window12(self, checks: dict) -> str:
        if os.environ.get("SET12_HAND"):
            # The owner leaves the guided setup that waits at its public-DNS step ("change the plan", then "set up
            # by hand"), so that the Domains list and the domain pages are reachable in the browser.
            checks["leave_setup"] = self.leave_setup()
        record = self.browser_window("ui", 3 * 3600, {"domains": [{"name": DOMAIN_A, "id": self.a_id()}]})
        checks.update(record)
        self.record_json("browser-window.json", record)
        if record.get("skipped"):
            return "skipped"
        if record.get("not_finished"):
            raise base.StepInconclusive(record["not_finished"])
        self.ensure_session()
        return "observed"

    def leave_setup(self) -> dict:
        shown = self.call("ui: GET /api/v1/setup", "GET", "/api/v1/setup", timeout=60)
        state = shown["_parsed"] if isinstance(shown["_parsed"], dict) else {}
        operation = self.call("ui: GET /api/v1/setup/operation (the latest setup execution)", "GET", "/api/v1/setup/operation", timeout=60)
        execution = operation["_parsed"] if isinstance(operation["_parsed"], dict) else {}
        out = {"status_before": state.get("status"), "guidance_before": state.get("guidance"), "phase": execution.get("phase"),
               "execution_status": execution.get("status")}
        if execution.get("status") == "waiting" and execution.get("id"):
            revised = self.call("ui: the owner reopens the plan (POST /api/v1/setup/revise)", "POST", "/api/v1/setup/revise",
                                {"revision": state.get("revision"), "execution_id": execution.get("id")}, timeout=120)
            out["revise"] = revised["status"]
            state = revised["_parsed"] if isinstance(revised["_parsed"], dict) else state
        guided = self.call("ui: the owner chooses to set up by hand (PUT /api/v1/setup/guidance manual)", "PUT",
                           "/api/v1/setup/guidance", {"revision": state.get("revision"), "guidance": "manual"}, timeout=60)
        out["guidance"] = guided["status"]
        after = self.call("ui: GET /api/v1/setup after", "GET", "/api/v1/setup", timeout=60)
        parsed = after["_parsed"] if isinstance(after["_parsed"], dict) else {}
        out.update(status_after=parsed.get("status"), guidance_after=parsed.get("guidance"))
        return out

    # -- the cell -----------------------------------------------------------------------------------------------

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("site", self.site, needs=("setup",))
        functions = {"S0-sites": self.s0_sites, "h-i-unchanged-start": self.h_i, "M-pool-owner-edit": self.m_pool,
                     "U-pool-not-read": self.u_not_read, "RN-take-nginx-refused": self.rn_nginx,
                     "RP1-take-php-fpm-test-refused": lambda: self.pool_refusal("rp1", "broken"),
                     "RP2-take-php-fpm-stopped": lambda: self.pool_refusal("rp2", "stopped"),
                     "P-apply-and-b4": self.p_apply, "A-probe-addresses": self.a_addresses, "N-app-unit": self.n_unit,
                     "L-pool-delete": self.l_delete}
        only = [k for k in os.environ.get("SET12_SECTIONS", "").split(",") if k]
        for key, title in SECTIONS:
            if only and key not in only and key not in ("S0-sites", "h-i-unchanged-start"):
                continue
            if key != "S0-sites" and not (self.s10.get("ref") and self.ids().get(DOMAIN_A)):
                self.step(key, lambda checks: checks.update(reason="S0 did not record the sites") or "not-run")
                continue
            self.section(key, title, functions[key], needs=("site",))
            if key != "S0-sites" and self.sections.get(key, {}).get("verdict") not in ("passed", "failed"):
                self.recover12(key)
        self.step("set12-browser-window", self.final_window12, needs=("site",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None, "provenance": {"what": "set12: 8fabc03b3 installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set12-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "set12": {k: v for k, v in self.s10.items() if k != "ref"} | {"s12": {k: v for k, v in self.s12.items() if k != "pool_ref"}},
                  "sections": self.sections, "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set12: a measurement on the fresh install; no update is started and no P0 row is judged."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)

    def recover12(self, label: str) -> None:
        """Lab recovery after a harness error inside a section: the owner's lab files taken back, PHP-FPM, nginx and
        the Panel started again, A's flags cleared."""
        for mode, arguments in (("lab-slow-reload", {"domain": DOMAIN_A, "remove": True}),
                                ("owner-broken-pool", {"domain": DOMAIN_A, "remove": True}),
                                ("owner-php12", {"domain": DOMAIN_A, "action": "start"}),
                                ("owner-pool-chattr", {"domain": DOMAIN_A, "flag": "-i"})):
            try:
                self.snap12(f"{label}-lab-recover-{mode}", mode, **arguments)
            except Exception as exc:  # noqa: BLE001
                self.current.setdefault("lab_preparation", []).append({"label": label, "mode": mode, "recovery_error": type(exc).__name__})
        self.recover11(label)


# ---------------------------------------------------------------------------
# Update cells
# ---------------------------------------------------------------------------

RN_DOMAIN = "set10-rn.test"


class Set12UpdateTrial(Set12Mixin, s10.Set10UpdateTrial):
    """set10's update cells; with the published alpha.82 as the baseline, the pool and the unit are prepared on it."""

    def __init__(self, *args: Any, **kwargs: Any) -> None:
        super().__init__(*args, **kwargs)
        self.ref = (self.artifacts.get("baseline_ref") or {}).get("ref")
        self.s10["baseline_ref"] = self.ref

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        for name in ("guest_set11_native.py", HELPER12):
            if name not in helpers:
                helpers[name] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / name, name)[1]
        if self.a82:
            # the owner-update observer's closed set of labels gains (alpha.82, alpha.83): set12_fixture's copy
            helpers["guest_bound_worker.py (set12 generated copy)"] = self.lab.put_file(
                self.root, self.record, self.plan, self.node_name, fx.bound_worker_copy(), "guest_bound_worker.py")[1]
        return helpers

    @property
    def a82(self) -> bool:
        return self.ref == fx.ALPHA82_REF

    def step(self, name: str, function, *, needs: tuple = ()) -> str:
        if self.a82 and name == "verdicts" and self.cell.variant == "good":
            # set10's database restore section expects alpha.81's ledger (42); not asked here: skipped by calling the
            # plain step of the parent's parent.
            return s10.s6.Set6UpdateTrial.step(self, name, function, needs=needs)
        return super().step(name, function, needs=needs)

    def project(self) -> str:
        return "php" if self.a82 or self.cell.node != "arch" else "static"

    def site_domains(self) -> list:
        return [s10.R1, s10.R2, s10.R3, "upd1-owner.test"]

    def create(self, domain: str) -> int:
        if not self.a82:
            return super().create(domain)
        created = self.call(f"create php site {domain} (alpha.82)", "POST", "/api/v1/domains/create",
                            {"domain": domain, "project_type": "php", "ssl_type": "none"}, timeout=600)
        answer = created["_parsed"] if isinstance(created["_parsed"], dict) else {}
        domain_id = answer.get("DomainID") or answer.get("domain_id")
        if created["status"] != 200 or not isinstance(domain_id, int):
            self.check(f"{domain} as a PHP site is created by alpha.82", False, created.get("answer") or answer)
            created = self.call(f"create static site {domain} (alpha.82; PHP was refused)", "POST", "/api/v1/domains/create",
                                {"domain": domain, "project_type": "static", "ssl_type": "none"}, timeout=600)
            answer = created["_parsed"] if isinstance(created["_parsed"], dict) else {}
            domain_id = answer.get("DomainID") or answer.get("domain_id")
        self.check(f"{domain} is created by alpha.82 (200 with its id)", created["status"] == 200 and isinstance(domain_id, int),
                   created.get("answer") or answer)
        if not isinstance(domain_id, int):
            raise base.StepFailed(f"{domain} was not created")
        self.s10["ids"][domain] = domain_id
        return domain_id

    def files_of(self, label: str, domains: list) -> dict:
        reading = self.snap10(f"{label}-files", "read-files10", domains=domains)
        out = {}
        for domain, site in (reading.get("sites") or {}).items():
            pools = site.get("pools") or {}
            for path, pool in pools.items():
                self.keep10(f"{label}-{domain}-{os.path.basename(path)}", pool)
            self.keep10(f"{label}-{domain}-unit", site.get("unit"))
            out[domain] = {"pools": {p: {k: v.get(k) for k in ("sha256", "inode", "mode", "uid", "gid", "first_line", "header")}
                                     for p, v in pools.items()},
                           "unit": {k: (site.get("unit") or {}).get(k) for k in ("exists", "sha256", "inode", "first_line", "header")}}
        return out

    def before_update(self) -> None:
        if not self.a82:
            super().before_update()
            return
        self.create(s10.R2)
        self.create(s10.R3)
        fired = self.panel10("pre", "restart")
        self.create(s10.R1)
        edit = self.owner10("pre-r3-edit", "owner-vhost10", domain=s10.R3, action="append-location", reload=True)
        pool_edit = None
        try:
            pool_edit = self.owner12("pre-r2-pool-edit", "owner-pool12", domain=s10.R2, action="append", reload=True)
        except Exception as exc:  # noqa: BLE001 - a site without a pool (static) is recorded
            self.note("R2's pool could not be edited", self.error_text(exc))
        node = self.node_site_before()
        reading = self.read10("pre-before-the-update", dump=True)
        domains = self.site_domains() + ([RN_DOMAIN] if node.get("site") else [])
        files = self.files_of("pre-before-the-update", domains)
        self.s10["before"] = {d: {k: reading["sites"][d]["vhost"].get(k) for k in ("sha256", "inode", "mtime_ns", "uid", "gid", "mode")}
                              | {"header": reading["sites"][d]["vhost"].get("header")} for d in self.site_domains()}
        self.s10["files_before"] = files
        self.s10["node"] = node
        self.s10["ids"].update({d["name"]: d["id"] for d in (reading.get("ledger") or {}).get("domains") or []})
        led = reading.get("ledger") or {}
        any_header = any((v.get("header") or {}).get("present") for v in self.s10["before"].values()) or any(
            (p.get("header") or {}).get("present") for f in files.values() for p in f["pools"].values()) or any(
            (f["unit"].get("header") or {}).get("present") for f in files.values())
        checks = [("the Panel restarted between R2/R3 and R1", bool(fired.get("ok")), fired),
                  ("the owner's edit of R3 is in service (nginx -t passed)", (edit.get("nginx_test") or {}).get("returncode") == 0,
                   edit.get("nginx_test")),
                  ("the owner's line in R2's pool passed php-fpm -t", bool(pool_edit) and (pool_edit.get("php_fpm_test") or {}).get("returncode") == 0,
                   (pool_edit or {}).get("php_fpm_test")),
                  ("alpha.82's files (vhosts, pools, unit) carry no render header", not any_header, files),
                  ("the ledger is at 43 before the update", led.get("schema_version") == 43, led.get("ledger_last"))]
        self.judge("B-before", checks, {"node": node})

    def node_site_before(self) -> dict:
        """On alpha.82: Node.js through the Panel and a Node site RN, when the release can."""
        try:
            listed = self.call("pre: Node.js versions (alpha.82)", "GET", "/api/v1/runtimes/node")
            installed = (listed["_parsed"] or {}).get("installed") if isinstance(listed["_parsed"], dict) else None
            version = installed[0] if installed else None
            if not version:
                posted = self.call(f"pre: install Node.js {NODE_FALLBACK} through alpha.82", "POST", "/api/v1/runtimes/node",
                                   {"version": NODE_FALLBACK, "request_id": secrets.token_hex(16)}, timeout=120)
                op = ((posted["_parsed"] or {}).get("operation") or {}) if isinstance(posted["_parsed"], dict) else {}
                deadline = time.time() + 900
                while op.get("id") and op.get("status") not in ("succeeded", "failed", "canceled", "cancelled") and time.time() < deadline:
                    time.sleep(10)
                    polled = self.api("GET", f"/api/v1/service/operation?id={op['id']}", purpose="pre: the install's state", timeout=30)
                    got = polled.json() if polled.status == 200 else {}
                    op = (got.get("operation") if isinstance(got, dict) and "operation" in got else got) or op
                listed = self.call("pre: Node.js versions after the install", "GET", "/api/v1/runtimes/node")
                installed = (listed["_parsed"] or {}).get("installed") if isinstance(listed["_parsed"], dict) else None
                version = NODE_FALLBACK if installed and NODE_FALLBACK in installed else None
                if not version:
                    return {"site": None, "why": "Node.js could not be installed through alpha.82", "operation": op}
            rn_id = self.create(RN_DOMAIN)
            command = "node -e \"require('http').createServer((q,s)=>s.end('set12-rn')).listen(process.env.PORT)\""
            hosting = self.call("pre: RN becomes a Node site (alpha.82)", "PUT", f"/api/v1/domains/{rn_id}/hosting",
                                {"project_type": "node", "start_command": command, "runtime_version": version}, timeout=600)
            return {"site": RN_DOMAIN if hosting["status"] == 200 else None, "version": version, "hosting": hosting["status"]}
        except Exception as exc:  # noqa: BLE001 - the unit's adoption is then not measured
            return {"site": None, "error": self.error_text(exc)}

    def after_update(self) -> None:
        if not self.a82:
            super().after_update()
            self.pools_after_a81()
            return
        good = self.cell.variant == "good"
        self.current["session"] = self.ensure_session()
        node_site = (self.s10.get("node") or {}).get("site")
        reading = self.read10("after-the-update", domains=self.site_domains() + self.extra_domains(), dump=True)
        domains = self.site_domains() + ([node_site] if node_site else [])
        files = self.files_of("after-the-update", domains)
        led = reading.get("ledger") or {}
        self.s10["ids"].update({d["name"]: d["id"] for d in led.get("domains") or []})
        since = self.state.get("started_at") or (time.time() - 3600)
        window = self.window("after-the-update", float(since) - 60)
        panel = window["by_unit"].get("celikpanel-panel.service") or []
        starts = s10.start_lines(panel)
        before = self.s10["before"]
        vhosts = {}
        for d in self.site_domains():
            v = reading["sites"][d]["vhost"]
            vhosts[d] = {"header": v.get("header"), "sha256": v.get("sha256"), "inode": v.get("inode"),
                         "same_bytes_as_before": v.get("sha256") == before[d]["sha256"],
                         "same_inode_as_before": v.get("inode") == before[d]["inode"],
                         "ledger_row": self.ledger_row(reading, self.s10["ids"].get(d)), "lines": s10.site_lines(panel, d)}
        fb = self.s10["files_before"]
        others = {d: {"pools_same": {p: (files.get(d, {}).get("pools", {}).get(p) or {}).get("sha256") == v.get("sha256")
                                     for p, v in fb.get(d, {}).get("pools", {}).items()},
                      "unit_same": (files.get(d, {}).get("unit") or {}).get("sha256") == (fb.get(d, {}).get("unit") or {}).get("sha256")}
                  for d in domains}
        self.current["vhosts"], self.current["other_files"], self.current["start_lines"] = vhosts, files, starts
        self.s10["after"] = vhosts
        if not good:
            alive = self.call("after the return: GET /api/v1/domains (the Panel usable)", "GET", "/api/v1/domains", timeout=120)
            self.judge("B-return", [
                ("the ledger is back at 43 (no managed_site_files table)",
                 led.get("schema_version") == 43 and led.get("managed_site_files_table") is False,
                 {k: led.get(k) for k in ("schema_version", "ledger_last", "managed_site_files_table")}),
                ("no vhost, pool or unit carries a render header",
                 not any((f["header"] or {}).get("present") for f in vhosts.values())
                 and not any((p.get("header") or {}).get("present") for f in files.values() for p in f["pools"].values())
                 and not any((f["unit"].get("header") or {}).get("present") for f in files.values()), {"vhosts": vhosts, "files": files}),
                ("pools and the unit are the bytes alpha.82 left (sha256)",
                 all(all(v["pools_same"].values()) and v["unit_same"] for v in others.values()), others),
                ("the Panel is usable after the return (the Domains list answers 200)", alive["status"] == 200, alive["status"]),
                ("the returned alpha.82 rendered the vhosts at its start (inodes recorded)", True,
                 {d: {"same_inode": f["same_inode_as_before"], "same_bytes": f["same_bytes_as_before"]} for d, f in vhosts.items()})],
                {"start_lines": starts})
            return
        rows = self.domains_rows("after the update")
        shown = {d: self.sc(f"after the update {d}", self.s10["ids"][d]) for d in [s10.R1, s10.R2, s10.R3] + ([node_site] if node_site else [])}
        r3_id = self.s10["ids"][s10.R3]
        probes = {}
        for action in ("keep", "take", "recreate", "apply"):
            probes[action] = self.post_raw(f"{action} without X-CelikPanel-Request-Id (a page loaded before the update)",
                                           f"/api/v1/domains/{r3_id}/site-config/{action}",
                                           {"file_sha256": shown[s10.R3]["body"].get("file_sha256"),
                                            "render_sha256": shown[s10.R3]["body"].get("render_sha256")}, None)
        later = self.read10("after-the-428-probes")
        rows2 = self.domains_rows("after the pages were read")
        adopted = [l for l in starts if "adopted, " in l and "left alone" in l]
        pool = lambda d: file_of(shown[d]["body"], "php_pool")
        unit = file_of(shown[node_site]["body"], "app_unit") if node_site else {}
        checks = [
            ("migration 044 applied (ledger 44, managed_site_files present)",
             led.get("schema_version") == 44 and led.get("managed_site_files_table") is True,
             {k: led.get(k) for k in ("schema_version", "ledger_last", "managed_site_files_table")}),
            ("R1 and R2 vhosts (alpha.82's text) adopted: header, ledger row adopted_from v0.1.0-alpha.82 (or its creation form)",
             all((vhosts[d]["header"] or {}).get("valid") and str((vhosts[d]["ledger_row"] or {}).get("adopted_from") or "").startswith(fx.ALPHA82_REF)
                 for d in (s10.R1, s10.R2)), {d: {k: vhosts[d][k] for k in ("header", "ledger_row", "lines")} for d in (s10.R1, s10.R2)}),
            ("R3 (owner-edited on alpha.82) is unknown_origin and was never written (sha256, inode)",
             vhosts[s10.R3]["same_bytes_as_before"] and vhosts[s10.R3]["same_inode_as_before"]
             and (vhosts[s10.R3]["ledger_row"] or {}).get("state") == "unknown_origin", vhosts[s10.R3]),
            ("the start line says N adopted, M left alone", bool(adopted), starts),
            ("pools and the unit are not rendered at the start (their bytes are alpha.82's)",
             all(all(v["pools_same"].values()) and v["unit_same"] for v in others.values()), others),
            ("R1's pool (alpha.82's text) is adopted: managed_unchanged, adopted_from v0.1.0-alpha.82",
             pool(s10.R1).get("state") == "managed_unchanged" and str(pool(s10.R1).get("adopted_from") or "").startswith(fx.ALPHA82_REF),
             {k: pool(s10.R1).get(k) for k in ("state", "adopted_from", "actions", "path")}),
            ("R2's pool (with the owner's line) is unknown origin", pool(s10.R2).get("state") == "unknown_origin",
             {k: pool(s10.R2).get(k) for k in ("state", "adopted_from", "actions")}),
            ("RN's unit (alpha.82's text) is adopted" if node_site else "a Node site's unit from alpha.82 is adopted",
             (unit.get("state") == "managed_unchanged" and str(unit.get("adopted_from") or "").startswith(fx.ALPHA82_REF)) if node_site else None,
             {k: unit.get(k) for k in ("state", "adopted_from", "actions")} if node_site else self.s10.get("node")),
            ("the four POST routes without an identity answer 428 REQUEST_ID_REQUIRED and change nothing",
             all(p["status"] == 428 and p["body"].get("code") == "REQUEST_ID_REQUIRED" for p in probes.values())
             and later["sites"][s10.R3]["vhost"].get("sha256") == vhosts[s10.R3]["sha256"],
             {a: {"status": p["status"], "code": p["body"].get("code")} for a, p in probes.items()}),
            ("the Domains list carries each site's state (R3 unknown_origin; R1 adopted); its first-state line is drawn",
             (rows.get(s10.R3) or {}).get("state") == "unknown_origin" and bool((rows.get(s10.R1) or {}).get("adopted_from"))
             and first_state_line(rows)["drawn"],
             {"rows": {d: rows.get(d) for d in self.site_domains() + ([node_site] if node_site else [])},
              "first_state_line": first_state_line(rows),
              "after_the_pages_were_read": {"rows": {d: rows2.get(d) for d in self.site_domains() + ([node_site] if node_site else [])},
                                            "first_state_line": first_state_line(rows2)}})]
        self.judge("B-forward", checks, {"pages": {d: self.page_brief(s["body"]) for d, s in shown.items()}})

    def pools_after_a81(self) -> None:
        """alpha.81 baseline: each PHP site's pool read through the page after the update (adoption of alpha.81's pool)."""
        out = {}
        for d in (s10.R1, s10.R2, s10.R3):
            shown = self.sc(f"after the update {d} (pool)", self.s10["ids"][d])
            out[d] = {"overall": shown["body"].get("overall_state"), "pool": {k: file_of(shown["body"], "php_pool").get(k)
                                                                             for k in ("state", "adopted_from", "path")}}
        self.s10["pools_after"] = out
        self.check("alpha.81's pools are adopted (managed_unchanged, adopted_from v0.1.0-alpha.81) or named",
                   all(v["pool"].get("state") == "managed_unchanged" and str(v["pool"].get("adopted_from") or "").startswith("v0.1.0-alpha.81")
                       for v in out.values()), out)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = {"schema": "celikpanel/set12-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES),
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation on the guest are recorded as such; no certificate authority and no "
                    "licence service is contacted by the driver"}
    if name in CELLS:
        settings = CELLS[name]
        plan.update(cell_kind=CELL_KIND, candidate={k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
                    setup={"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
                    steps=[s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup", "site"]
                          + [k for k, _ in SECTIONS] + ["set12-browser-window", s6.PIN_END_STEP, "collect"])
    else:
        cell = base.validate_cell(name)
        plan.update(cell_kind=UPDATE_KIND, baseline_ref=(artifacts.get("baseline_ref") or {}).get("ref"),
                    update=s6.build_plan(name, artifacts, work_root, local_port),
                    added_steps=[s10.PRE_STEP, s10.AFTER_STEP])
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
    fx.configure(document)
    cell = CELLS[args.cell].cell if args.cell in CELLS else base.validate_cell(args.cell)
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    if args.cell in CELLS:
        result = Set12Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
        print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                          "sections": {k: v.get("verdict") for k, v in result["sections"].items()}}, sort_keys=True))
    else:
        result = Set12UpdateTrial(cell, document, args.work_root, args.local_port).execute()
        print(json.dumps({"overall": result["overall"], "outcome": result["outcome"]["classification"],
                          "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
