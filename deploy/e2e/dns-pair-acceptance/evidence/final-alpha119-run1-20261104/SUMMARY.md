# alpha.119 lab acceptance, run 1 (2026-11-04, 01:12-01:55 UTC)

Candidate: `release/alpha119` at `bf708710e` (identity 119; previous 118 / `70c61f9d7`), built from the
remote ref via a bundle (no branches). Lab only; panel API like a user; SSH read-only except the lab
acting as a malicious customer (escape kit, uploads as the site's own account). The guest cells are named
`a118` (A, runs **119**) and `u118` (B); `raw/dist-119-candidate.json` / `raw/dist-118.json`.
**Stopped at the first product defect: item 4 (Python apps cannot be built).** Item 5 was not run.

| Item | Result | Evidence |
|---|---|---|
| 1 Guide | PASS | `raw/R7prep-a118.log`, `raw/R7cert-a118.log` |
| 2 Developer tools (Go) | PASS | `raw/R7go-a118.log` |
| 3 Uploaded program + escape matrix | PASS for the product (kit defects noted) | `raw/panel-runtime-program-a118.txt`, `raw/R7prog-a118.log`, `raw/R7runsh*-a118.log`, `raw/R7item18-a118.log` |
| 4 Python app | **FAIL — build never starts** | `raw/R7py-a118.log`, `raw/a119-python-build-failure.txt`, `raw/panel-runtime-python-a118.txt` |
| 5 n8n on gen 3 (A) | not run (stopped) | — |
| 6 B update 118→119 | PASS | `raw/Bupd-u118.log`, `raw/run-Bupd7.txt` |
| 7 B "needs the new sandbox" → apply | PASS | `raw/B7n8n-u118.log`, `raw/B7ref-u118.log`, `raw/B7smtp-u118.log` |
| 8 C Ubuntu 22.04 | not run: the lab kit has no 22.04 image (debian13, ubuntu2404, arch only) | — |

## Notes
1. `/help` and `/help/getting-started` 200 (SPA); built chunks admin/customer/reseller `.en` and chapters
   (getting-started, server-setup, team-members) served 200 as JavaScript. Guide state: customer
   `pending (v0)` → PUT `skipped (v1)` → `skipped`; admin `pending`. Portal guide: 53 pages
   (index + 26 en + 26 tr) and `manifest.sha256` verifies (the lab portal itself carries only get.sh and
   the release, it is not built with `build-download-portal.sh`).
2. Go from Components: `golang-go 2:1.24~2`, `go1.24.4`. Customer: `POST /service/install` and
   `GET /managed-services` → 403 `ADMIN_ONLY`.
3. Sandbox level **full** (no gaps; systemd 257, BPF, cgroup v2, nft socket cgroupv2).
   `runtime:program` offered; goprobe uploaded as `prog119_test` into app/bin; deploy succeeded
   (gen 3); HTTPS through the site 200; suspend → unit stopped (`failed` state, probe exit 143), resume →
   active; stop 200; logs 40 lines, secret hidden; remove ends with no app and no unit.
   Escape matrix (Debian 13, program; python items via run.sh):
   - panel-runtime.sh (real app): items 1-4, 6-10, 12-16, env, log PASS; item 9 counter +9.
   - run.sh (kit units, both probes): 1-4, 6-9, 11-17 PASS; 10 SKIP (host firewall drops both);
     18 PASS on substance (see kit defects); **no ESCAPED attempt anywhere.**
   - Item 5 has no result in either run: under the real unit the probe is PID 1 (PrivatePIDs) and its
     parent PID is 0, so goprobe tries nothing (kit gap; the program cannot see another process).
   **Kit defects found (lab-side fixes recorded as diffs in `raw/R7runsh2-a118.log`, `raw/R7item18-a118.log`):**
   - `panel-runtime.sh`: `counter()` under `pipefail` ends the script silently when the egress table is not
     loaded yet (first deploy) → `|| true`.
   - `run.sh`: `mark()` returns 1 after a detail-less PASS, so every `cond && mark N PASS || mark N FAIL`
     also marks FAIL (false FAILs for 9, 11, 17, 18) → `return 0`.
   - `run.sh` item 18: `nsenter -m test …` cannot exec inside the program's namespace (NoExecPaths) → read
     `/proc/<pid>/root…` instead.
   - `run.sh` item 18: `findmnt | grep 990002` matches the python test unit's name
     (`/run/celikpanel-sandbox/celikrt-990002.service/app`, source = 990001's own app), not the other site;
     the "stop left /run/celikpanel-sandbox/<unit>" check races the bind unit's stop (gone 2 s later).
     The helper refused the swapped folder (`app_folder_not_real` in `celikpanel-sandbox@celikrt-990001`),
     the running program still saw its own folder, and the unit started again with the real folder.
   Product observation: `control stop` on a created-but-never-deployed app answers 409
   `reason: systemctl` with systemd's raw text ("Unit celikrt-1.service not loaded") instead of a
   guided "not deployed" state.
4. **Defect.** Python installed from Components (needs the extra-package confirmation; done with
   `check_installs` + `confirmed_installs` from the preview) and `component:python:3.13` offered. Every
   Python deploy then fails at `build`: `verified_failure / build_failed`: "the build could not be started:
   Failed to find executable /var/www/celikpanel/subscriptions/4/sites/4/app/venv-1.partial/bin/python:
   No such file or directory". `app/` holds `.build-tmp .pip-cache config data src` but no
   `venv-1.partial`; nothing of a build unit is in the journal. Same for the wsgi probe (app 3) and the
   Flask app (app 4). Shown before the stop: a `requirements.txt` with `-e ./local-pkg` on line 2 is refused
   at `validate` with `requirements.txt:2 editable: -e ./local-pkg`. Not reached: the wsgi probe matrix,
   the Flask app from wheels, the broken-app fallback.
6. B (Ubuntu 24.04, BIND): 118 installed from the lab portal; update through the panel in 61 s
   (first start retried after `PANEL_UPDATE_BUSY`, as in 118 runs); migrations 057–063 present, services
   active, nginx -t ok, no panics.
7. n8n installed on 118 before the update (runs, owner login). After the update: admin
   `sandbox.needs_refresh: 1`, domain `install.needs_sandbox: true`; level **reduced, gap `bpf_missing`**
   (systemd 255); before the refresh no egress table and SMTP from the unit's cgroup connected to
   127.0.0.1:25. `POST /admin/service-apps/sandbox-refresh` → `started: [n8nb118.test]`; the unit now has
   `# celikpanel-sandbox-generation: 3`, `Requires=celikpanel-sandbox@celiksvc-1.service`,
   `BindPaths=/run/celikpanel-sandbox/celiksvc-1.service/app:…`, `EnvironmentFile=/etc/celikpanel-svcapps/
   celiksvc-1.env` (root 600, folder 700); egress table loaded, unit in `@smtp_blocked`; SMTP 25/465/587
   to 127.0.0.1, 192.0.2.25 and 1.1.1.1 from the unit's cgroup all refused; n8n HTTPS 200 and owner login.

Cleanup: guests, lab portal, dists, kit and work directory removed after the run.
