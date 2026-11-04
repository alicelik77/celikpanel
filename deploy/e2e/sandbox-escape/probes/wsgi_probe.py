"""Escape-test kit: the Python probe as a real CelikPanel Python app (README.md).

panel-runtime.sh (KIND=python) uploads this file and probe.py into
<site home>/app/src and deploys "wsgi_probe:application" with gunicorn
through the panel, exactly as a customer's Django or Flask app runs: the
site's own account, the base site sandbox, the venv the build made.

    GET /        200 (the deploy's health step)
    GET /matrix  starts the escape matrix once, in a thread (it takes longer
                 than a request may), and answers 202; the lines go to the
                 journal: SANDBOX-PROBE-RUN, -ENV, -ENVECHO, one line per
                 attempt, then SANDBOX-PROBE DONE
    GET /smtp    tries the mail ports again (after run.sh removed the table)

Test code only: it tries and reports, and keeps nothing an escape would give.
Lab VMs only.
"""

import os
import threading

import probe

_lock = threading.Lock()
_started = False


def _run():
    # The unit runs in <app>/src; the app folder is its parent.
    app_dir = os.path.dirname(os.getcwd())
    port = int(os.environ.get("PORT", "0") or 0)
    print("SANDBOX-PROBE-RUN " + os.environ.get("PROBE_RUN", "none"), flush=True)
    print(
        "SANDBOX-PROBE-ENV port=%d env_port=%s host=%s home=%s path=%s appdir=%s uid=%d"
        % (port, os.environ.get("PORT", ""), os.environ.get("HOST", ""), os.environ.get("HOME", ""),
           os.environ.get("PATH", ""), app_dir, os.getuid()),
        flush=True,
    )
    secret = os.environ.get("PROBE_SECRET", "")
    if secret:
        # The panel's log view must hide this value (the log check).
        print("SANDBOX-PROBE-ENVECHO " + secret, flush=True)
    try:
        probe.matrix(app_dir, port)
    finally:
        print("SANDBOX-PROBE DONE", flush=True)


def application(environ, start_response):
    global _started
    path = environ.get("PATH_INFO", "/")
    status = "200 OK"
    body = b"ok\n"
    if path == "/matrix":
        with _lock:
            if not _started:
                _started = True
                threading.Thread(target=_run, daemon=True).start()
        status, body = "202 Accepted", b"matrix started; read the log\n"
    elif path == "/smtp":
        probe.smtp()
        print("SANDBOX-PROBE SMTP-AGAIN DONE", flush=True)
    start_response(status, [("Content-Type", "text/plain"), ("Content-Length", str(len(body)))])
    return [body]
