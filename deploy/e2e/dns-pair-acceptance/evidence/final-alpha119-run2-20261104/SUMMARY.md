# alpha.119 lab acceptance, run 2 (2026-11-04, 02:18-02:35 UTC)

**Candidate:** `release/alpha119` at `245e107e4` (identity 119), built from the remote ref through
a bundle. The repo's escape kit was used unchanged.

**Guests:**
- A: Debian 13, cell `a118`. Installed with get.sh; setup succeeded.
- C: Arch, cell `c119`. Installed with get.sh; setup succeeded. Provisioned in parallel only.

**Result: stopped at the first product defect, A item 1. No Python app can be built.**

| Item | Result | Evidence |
|---|---|---|
| 1 Python (Flask, FastAPI, fallback, wsgi probe) | **FAIL** | `raw/R8py-a118.log`, `raw/a119r2-python-build-failure.txt` |
| 2 run.sh full matrix | not run (stopped) | — |
| 3 n8n gen 3 on A | not run (stopped) | — |
| 4 stop of a never-deployed program | not run (stopped) | — |
| 5 C (Arch) Python 3.14 and matrix items 9 and 18 | not run (stopped) | — |

## The defect

- **What happens:** every Python deploy fails at step `build` with `verified_failure / build_failed`:
  "the build could not be started: Failed to parse ProtectControlGroups: strict".
- **What was tried:** the Flask app (`wsgi`), the FastAPI app (`asgi`, uvicorn) and the Flask
  redeploy all fail the same way.
- **State left behind:** no `venv-*` is created, so there is no previous environment to fall back to.
  The sites still show the placeholder page.
- **Setup:** Python was installed from Components after confirming the preview's point updates, and
  `component:python:3.13` was offered.
- **Host:** systemd 257 (257.13-1~deb13u1).
- **Likely cause:** `internal/sitesandbox/directives.go:69/180` adds `ProtectControlGroups=strict`
  for systemd 257 and later. A unit *file* accepts that, and the uploaded-program units in run 1
  deployed with it. The build is started as a transient unit (`systemd-run -p …`), and systemd 257
  cannot parse `strict` as a transient property.
- **Never reached:** the wsgi probe matrix. Its run was stopped while it was starting.

## Cleanup

The guests, the lab portal, the dist, the kit and the work directory were removed.
