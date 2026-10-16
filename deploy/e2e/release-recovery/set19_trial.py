#!/usr/bin/env python3
"""set19: the owner's update from v0.1.0-alpha.90 to the alpha.92 candidate on Debian 13 when Certbot renewal is
the owner's moving workload, on disposable QEMU guests (lab.py), with a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set19.mjs, set12's browser window protocol).

The Frankfurt update of 2026-10-16 (alpha.89 -> alpha.90) died at its last step: turning certbot.timer back on
started certbot.service at once (Persistent=true, a missed elapse), and the scheduler restore required the service to
read exactly "inactive". Native records never caught it: fresh guests had never triggered the timer. set19 makes the
condition deterministic on the lab guest, then lets the owner update in the browser:

``set19-catchup-debian13``: the timer's persistent stamp (``/var/lib/systemd/timers/stamp-certbot.timer``) is dated
three days back while the timer is active, so the update's own timer start finds a missed elapse and starts the
renewal at once (Debian's RandomizedDelaySec is at most 12 h).
``set19-failed-debian13``: certbot.service is given a lab drop-in whose ExecStart fails and is started once, so the
renewal service is ``failed`` before the update (the owner's last renewal result).

LAB PREPARATION (not a product change), step ``set19-certbot-lab-preparation`` after the pre-update state and before
the owner's press: outbound TCP/UDP 443 is refused except to loopback (nftables, as set18; the release origin is the
guest's loopback and the certificate-authority names are pinned to loopback by set6's first step), then the cell's
preparation above. Every command and every reading before and after is recorded. Step ``set19-certbot-readings``
after the terminal state reads the renewal units, the stamp, certbot.service's journal since the preparation, the
update worker's and the recovery runner's journals, the request's recovery observation files and the reject
counters.

Every other step is set17's update cell (``set17_trial.Set17UpdateTrial``: the owner's press in the browser, the
page's own start held until the observer is armed, set16's and set6's update steps), except that set17's
post-update browser window ("ui") is not opened. Every result carries ``native_evidence: false``. Nothing here judges
a P0 row.

  set19_trial.py plan --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set19_trial.py run  --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import shlex
import sys
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set19_fixture as fx19  # noqa: E402  (registers the alpha.90 -> alpha.92 profile before the set17 chain reads it)
import set17_trial as t17  # noqa: E402  (set16/set15/set14 chain and set17_fixture)

s16, base, out = t17.s16, t17.base, t17.out
CELLS = {
    "set19-catchup-debian13": ("upd1-debian13-good", "catchup"),
    "set19-failed-debian13": ("upd1-debian13-good", "failed"),
}
for _name in CELLS:
    t17.UPDATE_CELLS[_name] = ("upd1-debian13-good", fx19.REF90, True)
    t17.BROWSER[_name] = {"lang": "en", "viewport": "desktop"}
STAMP = "/var/lib/systemd/timers/stamp-certbot.timer"
DROP_IN = "/etc/systemd/system/certbot.service.d/50-set19-lab-fail.conf"
NFT_TABLE = "cp_set19_lab"
READ_UNITS = ("set +e; for u in certbot.timer certbot.service; do echo \"== $u\"; systemctl show \"$u\" --no-pager "
              "-p LoadState -p UnitFileState -p ActiveState -p SubState -p Result -p InvocationID -p LastTriggerUSec "
              "-p NextElapseUSecRealtime -p ExecMainStartTimestamp -p ExecMainExitTimestamp -p ExecMainStatus "
              "-p DropInPaths -p NRestarts 2>&1; done; echo '== stamp'; stat -c '%n mtime=%y (%Y)' " + STAMP + " 2>&1; "
              "echo '== now'; date -u +%FT%TZ; date +%s; true")
NETWORK = ("set +e; command -v nft >/dev/null || DEBIAN_FRONTEND=noninteractive timeout 600 apt-get install -y -q nftables "
           ">/tmp/cp-set19-nft-install.log 2>&1; echo \"nft_install_rc=$?\"; command -v nft || exit 3; "
           f"nft add table inet {NFT_TABLE} && nft add chain inet {NFT_TABLE} out '{{ type filter hook output priority 0; policy accept; }}' && "
           f"nft add rule inet {NFT_TABLE} out oifname lo accept && "
           f"nft add rule inet {NFT_TABLE} out ip daddr 127.0.0.0/8 accept && "
           f"nft add rule inet {NFT_TABLE} out tcp dport 443 counter log prefix '\"cp-set19-443 \"' reject with tcp reset && "
           f"nft add rule inet {NFT_TABLE} out udp dport 443 counter reject; echo \"nft_rc=$?\"; "
           f"nft list table inet {NFT_TABLE}; echo '== self-test'; timeout 8 bash -c 'exec 3<>/dev/tcp/203.0.113.1/443' 2>&1; "
           "echo \"self_test_rc=$?\"; true")
PREPARATIONS = {
    "catchup": ("set +e; echo '== before'; " + READ_UNITS + "; echo '== age the persistent stamp (lab)'; "
                f"mkdir -p {shlex.quote(str(Path(STAMP).parent))}; touch -d '3 days ago' {STAMP}; echo \"touch_rc=$?\"; "
                "echo '== after'; " + READ_UNITS + "; echo '== timer definition'; systemctl cat certbot.timer 2>&1; true"),
    "failed": ("set +e; echo '== before'; " + READ_UNITS + "; echo '== failing ExecStart drop-in (lab)'; "
               f"mkdir -p {shlex.quote(str(Path(DROP_IN).parent))}; "
               f"printf '[Service]\\nExecStart=\\nExecStart=/bin/false\\n' > {DROP_IN}; echo \"write_rc=$?\"; cat {DROP_IN}; "
               "systemctl daemon-reload; echo \"reload_rc=$?\"; timeout 60 systemctl start certbot.service; "
               "echo \"start_rc=$? (expected non-zero)\"; echo '== after'; " + READ_UNITS + "; true"),
}


class Set19UpdateTrial(t17.Set17UpdateTrial):
    def __init__(self, cell: Any, artifacts: dict, work_root: str, local_port: int, *, name: str, kind: str) -> None:
        super().__init__(cell, artifacts, work_root, local_port, name=name, page_window=True)
        self.kind = kind
        self.s19: dict = {}

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers.pop("guest_bound_worker.py (set17 generated copy)", None)
        helpers["guest_bound_worker.py (set19 generated copy)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, fx19.bound_worker_copy(), "guest_bound_worker.py")[1]
        return helpers

    def lab_preparation(self, checks: dict) -> str:
        """Lab preparation (not a product change): refuse outbound 443, then the cell's Certbot condition."""
        self.s19["prepared_epoch"] = time.time()
        network = self.guest(NETWORK, timeout=900)
        self.ev.write_text(f"{self.step_dir}/set19-lab-network.txt", (network.stdout or "") + (network.stderr or ""))
        prepared = self.guest(PREPARATIONS[self.kind], timeout=180)
        text = (prepared.stdout or "") + (prepared.stderr or "")
        self.ev.write_text(f"{self.step_dir}/set19-certbot-preparation.txt", text)
        after = text.split("== after", 1)[-1]
        checks.update(kind=self.kind, at=base.utc_now(), network_returncode=network.returncode,
                      reject_rule="nft_rc=0" in (network.stdout or ""),
                      self_test_refused="self_test_rc=0" not in (network.stdout or ""),
                      preparation_returncode=prepared.returncode,
                      what=("lab preparation (not a product change): outbound 443 refused except loopback; " +
                            ("the persistent stamp of certbot.timer dated three days back, the timer left active"
                             if self.kind == "catchup" else
                             "certbot.service given a failing ExecStart drop-in and started once (failed)")))
        if self.kind == "catchup":
            checks["stamp_aged"] = "touch_rc=0" in text
            checks["timer_active_after"] = "ActiveState=active" in after.split("== certbot.service")[0]
            ok = checks["stamp_aged"] and checks["timer_active_after"]
        else:
            service = after.split("== certbot.service", 1)[-1]
            checks["service_failed_before_update"] = "ActiveState=failed" in service
            checks["drop_in_loaded"] = DROP_IN in service
            ok = checks["service_failed_before_update"] and checks["drop_in_loaded"]
        self.s19["preparation"] = checks
        self.record_json("set19-certbot-preparation.json", checks)
        if not (ok and checks["reject_rule"]):
            raise base.StepFailed(f"the set19 lab preparation did not take effect: {checks}")
        return "passed"

    def certbot_readings(self, checks: dict) -> str:
        """Read-only: the renewal units after the update, the journals and the request's recovery observations."""
        rid = self.state.get("request_id") or ""
        since = int(self.s19.get("prepared_epoch") or (time.time() - 3600))
        units = self.guest(READ_UNITS, timeout=60)
        self.ev.write_text(f"{self.step_dir}/certbot-units-after.txt", (units.stdout or "") + (units.stderr or ""))
        journals = ("set +e; echo '== certbot.service since the preparation'; journalctl -u certbot.service --no-pager "
                    f"-o short-iso --since @{since} 2>&1 | tail -n 80; ")
        if t17.RID.fullmatch(rid):
            journals += (f"echo '== update worker (celikpanel-self-update-{rid})'; journalctl -u celikpanel-self-update-{rid}.service "
                         "--no-pager -o short-iso 2>&1 | tail -n 120; "
                         f"echo '== recovery runner since the preparation'; journalctl -u celikpanel-release-recovery.service "
                         f"--no-pager -o short-iso --since @{since} 2>&1 | tail -n 120; "
                         f"echo '== recovery observations of the request'; for f in /var/lib/celikpanel-recovery-observations/{rid}.*; "
                         "do echo \"-- $f\"; cat \"$f\" 2>&1; done; ")
        journals += f"echo '== 443 reject counters'; nft list table inet {NFT_TABLE} 2>&1; true"
        read = self.guest(journals, timeout=120)
        text = (read.stdout or "") + (read.stderr or "")
        self.ev.write_text(f"{self.step_dir}/journals-and-observations.txt", text)
        after = units.stdout or ""
        timer, service = after.split("== certbot.service", 1) if "== certbot.service" in after else (after, "")
        value = lambda block, key: next((line.split("=", 1)[1] for line in block.splitlines() if line.startswith(key + "=")), None)  # noqa: E731
        checks.update(
            request_id=rid,
            timer={"UnitFileState": value(timer, "UnitFileState"), "ActiveState": value(timer, "ActiveState"),
                   "LastTriggerUSec": value(timer, "LastTriggerUSec"), "NextElapseUSecRealtime": value(timer, "NextElapseUSecRealtime")},
            service={"ActiveState": value(service, "ActiveState"), "SubState": value(service, "SubState"),
                     "Result": value(service, "Result"), "InvocationID": value(service, "InvocationID"),
                     "ExecMainStartTimestamp": value(service, "ExecMainStartTimestamp"),
                     "ExecMainExitTimestamp": value(service, "ExecMainExitTimestamp"),
                     "ExecMainStatus": value(service, "ExecMainStatus")},
            renewal_started_after_preparation="Starting certbot" in text or "certbot.service: Deactivated" in text
            or "Finished certbot" in text or "Failed to start certbot" in text,
            update_failure_line="CELIKPANEL_UPDATE_FAILURE" in text,
            failure_sidecar=f"{rid}.failure" in text,
            reject_counter_line=next((line.strip() for line in text.splitlines() if "dport 443" in line and "counter" in line), None))
        checks["timer_enabled_and_active"] = (checks["timer"]["UnitFileState"] == "enabled"
                                              and checks["timer"]["ActiveState"] == "active")
        self.record_json("set19-certbot-readings.json", checks)
        if not checks["timer_enabled_and_active"]:
            self.finding(f"{self.set17_name}: certbot.timer is not enabled and active after the update: {checks['timer']}")
            return "failed"
        return "observed"

    def step(self, name: str, function, *, needs: tuple = ()) -> str:
        if name == "arm":
            base.Trial.step(self, "set19-certbot-lab-preparation", self.lab_preparation, needs=("pre-state",))
            needs = tuple(needs) + ("set19-certbot-lab-preparation",)
        if name == "collect":
            label = "after-the-update" if self.cell.variant == "good" else "after-the-return"
            base.Trial.step(self, f"set17-readings-{label}", lambda checks: self.readings17(checks, label),
                            needs=("terminal",))
            base.Trial.step(self, "set19-certbot-readings", self.certbot_readings, needs=("terminal",))
        # set17's post-update browser window ("ui") is not opened in set19.
        return super(s16.Set16UpdateTrial, self).step(name, function, needs=needs)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = t17.build_plan(name, artifacts, work_root, local_port)
    kind = CELLS[name][1]
    plan.update(schema="celikpanel/set19-plan/v1", source_commit=artifacts.get("source_head"),
                added_steps=["set19-certbot-lab-preparation", "set17-readings-after-the-update", "set19-certbot-readings"],
                not_run=list(plan.get("not_run") or []) + ["set17's post-update browser window (ui)"],
                lab_preparation={"kind": kind, "network": f"nftables table inet {NFT_TABLE}: outbound TCP/UDP 443 refused "
                                                          "except loopback",
                                 "certbot": PREPARATIONS[kind]})
    return plan


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
    labels = fx19.configure(document)
    if labels.get("ref") != fx19.REF90:
        parser.error(f"cell {args.cell} needs an artifacts document with baseline_ref {fx19.REF90}, not {labels.get('ref')}")
    fx19.SOURCE_COMMIT = document.get("source_head")
    base_cell, kind = CELLS[args.cell]
    cell = base.validate_cell(base_cell)
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    result = Set19UpdateTrial(cell, document, args.work_root, args.local_port, name=args.cell, kind=kind).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
