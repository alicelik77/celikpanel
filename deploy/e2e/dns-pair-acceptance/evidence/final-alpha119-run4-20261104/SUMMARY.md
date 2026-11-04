# alpha.119 lab acceptance, run 4 (2026-11-04, 03:24-03:55 UTC)

Candidate: `release/alpha119` at `8b52c9d39` (identity 119). Built from the remote ref through a
bundle; the repo's escape kit was used unchanged.

Guests:
- A: Debian 13, systemd 257, cell `a118`
- C: Arch, systemd 262, cell `c119`

Both were installed with get.sh and finished setup. Lab only, panel API like a user; SSH read-only
except the kit, which acts as the malicious customer.

**No product defect stopped the run.** One item does not match the brief: item 1c, see its note.

| Item | Result | Evidence |
|---|---|---|
| 1a Flask over HTTPS (wheels, `flask==3.1.2`) | PASS | `raw/R8py-a118.log` |
| 1b FastAPI / uvicorn (asgi, `fastapi==0.115.6`) | PASS | `raw/R8py-a118.log` |
| 1c broken-module redeploy | falls back as documented; **does not keep answering** | `raw/R8py-a118.log`, `raw/R8fallback-a118.log` |
| 1d `wsgi_probe` matrix | PASS | `raw/panel-runtime-python-a118.txt` |
| 2 `run.sh` full matrix | PASS | `raw/R7runsh-a118.log` |
| 3 n8n on gen 3 | PASS | `raw/R7n8n-a118.log`, `raw/R7n8nrun-a118.log` |
| 4 stop of a never-deployed program | PASS | `raw/R8stop-a118.log` |
| 5 Arch Python 3.14 Flask | PASS | `raw/R8arch-c119.log` |
| 6 Arch uploaded program, items 9 and 18 | PASS | `raw/panel-runtime-program-c119.txt`, `raw/R8arch-c119.log` |

## Matrix results

| Probe / OS | 1-9 | 10 | 11 | 12-16 | 17 | 18 |
|---|---|---|---|---|---|---|
| Debian 13, `run.sh` full (both probes) | PASS | SKIP | PASS | PASS (14 SKIP for python) | PASS | PASS |
| Debian 13, wsgi probe as a real Python app | PASS | PASS | run.sh | PASS (14 SKIP) | run.sh | — |
| Arch, real uploaded program (`panel-runtime.sh`) | PASS (9: counter +9) | PASS | run.sh | PASS | run.sh | — |
| Arch, `run.sh quick` (both probes) | PASS | PASS | not in quick | PASS | not in quick | PASS |

Notes on the matrix:
- No attempt reported ESCAPED.
- Items 5 and 18 gave real results.
- Item 10 on Debian is SKIP because the host firewall drops both the sandboxed listener and the
  reference listener, so the result is inconclusive.
- "run.sh" in a cell means that item is only checked by `run.sh`, not through the real app.

## Item notes

**1a-1b.** Flask returned "lab flask app ok". FastAPI returned `{"lab":"fastapi app ok"}`.

**1c.** The redeploy built `venv-2`, then failed at `proof` with `verified_failure / start_failed`:
"…The app was put back on its previous environment (venv-1) and restarted; the code in app/src is
what you uploaded".
- The unit now runs `venv-1/bin/python -m gunicorn`.
- The uploaded `app.py` itself raises, so gunicorn exits again: 502, state `not_listening`.
- This matches RUNTIME-APPS (the fallback restores the environment, not the code).
- So "keeps answering" cannot hold for a broken module. Owner decision.

**3.** Node.js 24.22.2 was installed and n8n offered; n8n 2.46.4 then installed through the
transient install unit in about 9 minutes.
- Unit and sandbox: unit `# celikpanel-sandbox-generation: 3`,
  `Requires`/`After=celikpanel-sandbox@celiksvc-1.service` (active), and
  `BindPaths=/run/celikpanel-sandbox/celiksvc-1.service/app:…`.
- Environment file: `EnvironmentFile=/etc/celikpanel-svcapps/celiksvc-1.env` (root 600, folder 700).
- Running: the unit listens on 127.0.0.1:21000/21001 only, HTTPS returns 200, and the owner login
  returns 200 (`global:owner`).
- Mail ports: SMTP 25/465/587 to 127.0.0.1, 192.0.2.25 and 1.1.1.1 from inside its cgroup were
  all refused. The admin view shows `smtp_rejects 27`, level full.

**4.** `control stop` on a never-deployed app answered 200 with `reason: not_deployed`, `state:
created` and a plain message. `start` answered 409 `not_deployed`.

**5.** Python 3.14.9 from Components, `component:python:3.14` offered; the deploy succeeded and the
site answered.

**Lab wrapper fix.** My `prt.sh` used `hostname -I`, which Arch does not have. I fixed it and reran
item 6; this was not a product issue.

**Not observed.** The noted 30 s stop of a program without a SIGTERM handler was not timed in this
run.

**Cleanup.** The guests, the lab portal, the dist, the kit and the work directory were removed.
