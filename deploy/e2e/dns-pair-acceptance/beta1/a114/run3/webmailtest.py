#!/usr/bin/env python3
"""Run on labnet: log in to Roundcube at https://HOST/ as USER (password on
stdin JSON key "to") and list INBOX the way the page's XHR does; report the
test mails' subjects seen. The password is never printed. Lab only.
usage: webmailtest.py HOST USER < {"to": pw}"""
import http.cookiejar, json, re, socket, ssl, subprocess, sys, urllib.parse, urllib.request
host, user = sys.argv[1:3]
pw = json.load(sys.stdin)["to"]
_real = socket.getaddrinfo
def _gai(name, *a, **k):
    if isinstance(name, str) and name.endswith(".test"):
        ans = subprocess.run(["dig", "+short", "@192.0.2.53", name, "A"], capture_output=True, text=True).stdout.split()
        name = [x for x in ans if x[:1].isdigit()][-1]
    return _real(name, *a, **k)
socket.getaddrinfo = _gai
ctx = ssl.create_default_context(cafile="/root/beta1/pebble-root.pem")
jar = http.cookiejar.CookieJar()
op = urllib.request.build_opener(urllib.request.HTTPSHandler(context=ctx), urllib.request.HTTPCookieProcessor(jar))
base = f"https://{host}/"
out = {}
def token(html):
    m = re.search(r'"request_token":"([^"]+)"', html)
    return m.group(1) if m else None
try:
    r = op.open(base, timeout=30); html = r.read().decode("utf-8", "replace")
    out["login_page"] = r.status
    t = token(html)
    data = urllib.parse.urlencode({"_token": t, "_task": "login", "_action": "login", "_timezone": "UTC",
                                   "_url": "", "_user": user, "_pass": pw}).encode()
    r = op.open(urllib.request.Request(base + "?_task=login", data=data), timeout=30)
    html = r.read().decode("utf-8", "replace")
    out["after_login_url"] = re.sub(r"_token=[^&]*", "_token=[REDACTED]", r.geturl().replace(host, "HOST"))
    out["logged_in"] = '"task":"mail"' in html
    t2 = token(html)
    req = urllib.request.Request(base + "?_task=mail&_action=list&_mbox=INBOX&_remote=1&_refresh=1",
                                 headers={"X-Roundcube-Request": t2 or "", "X-Requested-With": "XMLHttpRequest"})
    r = op.open(req, timeout=30); body = r.read().decode("utf-8", "replace")
    subj = sorted(set(re.findall(r'final-alpha113 [0-9a-f]{12}', body)))
    out["inbox_list_status"] = r.status
    out["test_mails_listed"] = subj
    out["message_count_hint"] = (re.search(r'set_rowcount\(\\?"([^"\\]*)', body) or [None, None])[1]
except Exception as e:
    out["error"] = repr(e)[:400]
print(json.dumps(out, indent=1))
