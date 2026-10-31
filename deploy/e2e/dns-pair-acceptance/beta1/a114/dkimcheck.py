#!/usr/bin/env python3
"""Run on labnet: fetch the test mails (by subject) from bob's INBOX over IMAPS
and verify each DKIM signature independently (dkimpy) against the
<s>._domainkey.<d> TXT as served, without recursion, by EACH given DNS server.
Also lists the scan/authentication headers. Password on stdin {"to": pw}.
usage: dkimcheck.py IMAPHOST USER DNS_IP[,DNS_IP] SUBJECT...   Lab only."""
import imaplib, json, re, socket, ssl, subprocess, sys
import dkim
host, user, ips = sys.argv[1], sys.argv[2], sys.argv[3].split(",")
subjects = sys.argv[4:]
pw = json.load(sys.stdin)["to"]
_real = socket.getaddrinfo
def _gai(name, *a, **k):
    if isinstance(name, str) and name.endswith(".test"):
        ans = subprocess.run(["dig", "+short", "@192.0.2.53", name, "A"], capture_output=True, text=True).stdout.split()
        name = [x for x in ans if x[:1].isdigit()][-1]
    return _real(name, *a, **k)
socket.getaddrinfo = _gai
ctx = ssl.create_default_context(cafile="/root/beta1/pebble-root.pem")

def txt_at(ip, name):
    out = subprocess.run(["dig", "+short", "+norecurse", "+time=3", "+tries=1", f"@{ip}", name, "TXT"],
                         capture_output=True, text=True).stdout
    return "".join(re.findall(r'"((?:[^"\\]|\\.)*)"', out))

res = {"dns_servers": ips, "mails": []}
m = imaplib.IMAP4_SSL(host, 993, ssl_context=ctx, timeout=30)
m.login(user, pw)
m.select("INBOX", readonly=True)
for subj in subjects:
    entry = {"subject": subj}
    typ, data = m.search(None, "SUBJECT", f'"{subj}"')
    ids = data[0].split()
    if not ids:
        entry["found"] = False; res["mails"].append(entry); continue
    typ, msg = m.fetch(ids[-1], "(BODY.PEEK[])")
    raw = msg[0][1]
    head = raw.split(b"\r\n\r\n", 1)[0].decode("utf-8", "replace")
    unfolded = re.sub(r"\r\n[ \t]+", " ", head)
    hdr = lambda n: [l.split(":", 1)[1].strip() for l in unfolded.split("\r\n") if l.lower().startswith(n.lower() + ":")]
    entry["message_id"] = (hdr("Message-ID") or [None])[0]
    entry["from"] = (hdr("From") or [None])[0]
    sigs = hdr("DKIM-Signature")
    entry["dkim_signatures"] = len(sigs)
    entry["scan_headers"] = [l[:160] for l in unfolded.split("\r\n") if re.match(r"(?i)(x-spam|x-rspamd|x-spamd|authentication-results)", l)]
    checks = []
    for s in sigs:
        tags = dict(t.strip().split("=", 1) for t in s.split(";") if "=" in t)
        d, sel = tags.get("d", "").strip(), tags.get("s", "").strip()
        rec = {"d": d, "s": sel, "a": tags.get("a", "").strip(), "c": tags.get("c", "").strip(), "h": re.sub(r"\s+", "", tags.get("h", ""))[:120]}
        for ip in ips:
            txt = txt_at(ip, f"{sel}._domainkey.{d}")
            ok = None
            if txt:
                try:
                    ok = bool(dkim.verify(raw, dnsfunc=lambda name, timeout=5, t=txt: t.encode()))
                except Exception as e:
                    ok = f"error {e!r}"[:200]
            rec[f"verify_with_txt_from_{ip}"] = ok if txt else "no TXT served"
            rec[f"txt_p_prefix_{ip}"] = (re.search(r"p=([A-Za-z0-9+/]{0,24})", txt) or [None, None])[1]
        checks.append(rec)
    entry["dkim"] = checks
    res["mails"].append(entry)
m.logout()
print(json.dumps(res, indent=1))
