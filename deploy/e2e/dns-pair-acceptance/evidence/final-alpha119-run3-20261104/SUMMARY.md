# alpha.119 lab acceptance, run 3 (2026-11-04, 02:44-03:05 UTC)

**Candidate:** `release/alpha119` at `7b43c2d05` (identity 119), built from the remote ref through
a bundle. The repo's escape kit was used unchanged.

**Guests:**
- A: Debian 13, cell `a118`. Installed with get.sh; setup succeeded.
- C: Arch, cell `c119`. Installed with get.sh; setup succeeded. Provisioned in parallel only.

**Result: stopped at the first product defect, A item 1. Python builds still never run.**

| Item | Result | Evidence |
|---|---|---|
| 1 Python (Flask, FastAPI, fallback, wsgi probe) | **FAIL** | `raw/R8py-a118.log`, `raw/a119r3-python-build-failure.txt` |
| 2 run.sh full matrix | not run (stopped) | — |
| 3 n8n gen 3 | not run (stopped) | — |
| 4 stop of a never-deployed program | not run (stopped) | — |
| 5-6 C (Arch) | not run (stopped) | — |

## The defect

The `ProtectControlGroupsEx` parse error is gone, but the build still never runs.

**What the panel showed**
- First deploy of Flask (app 1) and of FastAPI (app 2) ended at `build` with `class: unknown`:
  "the build stopped without a result (the server restarted or someone stopped it); the running
  app was not changed. Deploy again".
- The Flask redeploy then failed with `verified_failure`: "the build could not be started: Failed
  to start transient service unit: Unit celikrtbuild-1.service was already loaded or has a
  fragment file."
- The wsgi probe deploy (app 3) did not finish either. The kit's log read failed with
  `lines: null`.
- No `venv-*` is ever created, so the broken-module fallback could not be tested.

**What the guest showed** (`raw/a119r3-python-build-failure.txt`)
- **Build unit never started.** `celikrtbuild-1.service` has `Type=oneshot`, `RemainAfterExit=yes`
  and `Requires=` plus `After=celikpanel-sandbox@celikrtbuild-1.service`. Its state is
  `loaded inactive dead`, it has an empty `ExecMainStartTimestamp` and no InvocationID, so
  `python3.13 -m venv …/venv-1.partial` never ran.
- **Bind unit stopped right after it started.** `celikpanel-sandbox@celikrtbuild-1.service` went
  "Starting" at 02:56:45.882, then "Main process exited, code=killed, status=15/TERM" and
  "Stopped" 42 ms later (`Result=signal`). The required dependency failed, so the build job never
  ran.
- **The empty unit stays loaded.** It was never collected, so every later deploy is refused with
  "already loaded".

## Cleanup

The guests, the lab portal, the dist, the kit and the work directory were removed.
