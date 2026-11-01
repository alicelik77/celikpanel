#!/usr/bin/env python3
"""labnet (the lab's internet) mail helper for the alpha.114 acceptance. Secrets come on stdin as JSON.
  send HOST FROM TO SUBJECT          stdin {"password": pw}   587 STARTTLS+AUTH, prints {"sent":..}
  wait HOST USER REGEX SECONDS       stdin {"password": pw}   IMAPS: waits for messages whose Subject matches,
                                                             prints subjects/from/dates and any account links
                                                             (link tokens only to the 0600 file given by LINKS env)
Lab only."""
import email, imaplib, json, os, re, smtplib, socket, ssl, subprocess, sys, time, uuid
from email.header import decode_header, make_header

CA = "/root/beta1/pebble-root.pem"
_real = socket.getaddrinfo
_cache = {}


def _gai(name, *a, **k):
    if isinstance(name, str) and name.endswith(".test"):
        if name not in _cache:
            ans = subprocess.run(["dig", "+short", "@192.0.2.53", name, "A"], capture_output=True, text=True).stdout.split()
            _cache[name] = [x for x in ans if x[:1].isdigit()][-1]
        name = _cache[name]
    return _real(name, *a, **k)


socket.getaddrinfo = _gai
ctx = ssl.create_default_context(cafile=CA)
cmd = sys.argv[1]
pw = json.load(sys.stdin)["password"]
if cmd == "send":
    host, frm, to, subject = sys.argv[2:6]
    s = smtplib.SMTP(host, 587, timeout=30); s.ehlo("labnet.test"); s.starttls(context=ctx); s.ehlo("labnet.test"); s.login(frm, pw)
    msg = (f"From: {frm}\r\nTo: {to}\r\nSubject: {subject}\r\nMessage-ID: <{uuid.uuid4().hex}@labnet.test>\r\n"
           f"Date: {email.utils.formatdate()}\r\n\r\nalpha.114 acceptance mail\r\n")
    refused = s.sendmail(frm, [to], msg); s.quit()
    print(json.dumps({"sent": not refused, "subject": subject, "from": frm, "to": to}))
elif cmd == "wait":
    host, user, rx, secs = sys.argv[2], sys.argv[3], re.compile(sys.argv[4]), float(sys.argv[5])
    deadline = time.time() + secs
    found, links = [], []
    while True:
        m = imaplib.IMAP4_SSL(host, 993, ssl_context=ctx); m.login(user, pw); m.select("INBOX", readonly=True)
        typ, data = m.search(None, "ALL")
        found, links = [], []
        for num in data[0].split():
            typ, raw = m.fetch(num, "(RFC822)")
            msg = email.message_from_bytes(raw[0][1])
            subj = str(make_header(decode_header(msg.get("Subject", ""))))
            if not rx.search(subj):
                continue
            body = ""
            for part in msg.walk():
                if part.get_content_type() in ("text/plain", "text/html"):
                    body += (part.get_payload(decode=True) or b"").decode("utf-8", "replace")
            ls = re.findall(r"https://[^\s\"'<>]+/account-link/[a-z]+#t=[A-Za-z0-9_-]+", body)
            links += ls
            found.append({"subject": subj, "from": msg.get("From"), "to": msg.get("To"), "date": msg.get("Date"),
                          "auto_submitted": msg.get("Auto-Submitted"), "dkim": bool(msg.get("DKIM-Signature")),
                          "link_shapes": [re.sub(r"#t=.*", "#t=[REDACTED]", x) for x in ls]})
        m.logout()
        if found or time.time() > deadline:
            break
        time.sleep(5)
    print(json.dumps({"user": user, "count": len(found), "messages": found[-6:], "links": links}))
