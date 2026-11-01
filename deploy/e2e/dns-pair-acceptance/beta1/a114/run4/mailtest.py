#!/usr/bin/env python3
"""Run on the labnet guest (the lab's "internet"): SMTP submission 587 with
STARTTLS + AUTH from one mailbox to the other, then read it over IMAPS 993.
Mailbox passwords come on stdin as JSON (never on the command line). Also
reports the certificate names served on 587/993/465 for several SNI names.
usage: mailtest.py HOST FROM TO [SNI...] < {"from": pw, "to": pw}. Lab only."""
import imaplib, json, smtplib, socket, ssl, sys, time, uuid
host, frm, to = sys.argv[1:4]; snis = sys.argv[4:]
pw = json.load(sys.stdin)
ca = "/root/beta1/pebble-root.pem"
ctx = ssl.create_default_context(cafile=ca)
import subprocess as _sp
# labnet's own resolv.conf is systemd-resolved; the lab's "public" resolver is unbound @192.0.2.53
_real_gai = socket.getaddrinfo
_cache = {}
def _lab_gai(name, *a, **k):
    if isinstance(name, str) and name.endswith(".test"):
        if name not in _cache:
            ans = _sp.run(["dig", "+short", "@192.0.2.53", name, "A"], capture_output=True, text=True).stdout.split()
            _cache[name] = [x for x in ans if x[:1].isdigit()][-1]
        name = _cache[name]
    return _real_gai(name, *a, **k)
socket.getaddrinfo = _lab_gai
out = {"host": host, "resolved": _lab_gai(host, 0)[0][4][0]}

def names(cert):
    subj = dict(x[0] for x in cert.get("subject", ()))
    return {"cn": subj.get("commonName"), "san": [v for k, v in cert.get("subjectAltName", ()) if k == "DNS"],
            "not_after": cert.get("notAfter"), "issuer_cn": dict(x[0] for x in cert.get("issuer", ())).get("commonName")}

def peek(port, sni):
    c = ssl.create_default_context(cafile=ca); c.check_hostname = False; c.verify_mode = ssl.CERT_NONE
    try:
        if port == 587:
            s = smtplib.SMTP(out["resolved"], 587, timeout=20); s.ehlo("labnet.test")
            s.docmd("STARTTLS")
            tls = c.wrap_socket(s.sock, server_hostname=sni)
            der = tls.getpeercert(binary_form=True); tls.close()
        else:
            raw = socket.create_connection((out["resolved"], port), timeout=20)
            tls = c.wrap_socket(raw, server_hostname=sni); der = tls.getpeercert(binary_form=True); tls.close()
        pem = ssl.DER_cert_to_PEM_cert(der)
        import subprocess
        txt = subprocess.run(["openssl", "x509", "-noout", "-subject", "-ext", "subjectAltName", "-issuer", "-enddate"], input=pem.encode(), capture_output=True).stdout.decode()
        return " ".join(txt.split())
    except Exception as e:
        return f"error: {e!r}"

subject = "final-alpha113 " + uuid.uuid4().hex[:12]
t0 = time.time()
try:
    s = smtplib.SMTP(host, 587, timeout=30)
    s.ehlo("labnet.test")
    s.starttls(context=ctx)  # verifies the chain (pebble root) and the host name
    out["smtp_587_cert"] = names(s.sock.getpeercert())
    s.ehlo("labnet.test")
    s.login(frm, pw["from"])
    msg = f"From: {frm}\r\nTo: {to}\r\nSubject: {subject}\r\nMessage-ID: <{uuid.uuid4().hex}@labnet.test>\r\n\r\nfinal alpha.113 acceptance test mail\r\n"
    refused = s.sendmail(frm, [to], msg)
    out["smtp_send"] = {"ok": not refused, "refused": refused}
    s.quit()
except Exception as e:
    out["smtp_send"] = {"ok": False, "error": repr(e)}
subject465 = subject.replace("final-alpha113 ", "final-alpha113-465 ")
try:
    s5 = smtplib.SMTP_SSL(host, 465, timeout=30, context=ctx)
    s5.login(frm, pw["from"])
    msg5 = "\r\n".join([f"From: {frm}", f"To: {to}", f"Subject: {subject465}",
                        f"Message-ID: <{uuid.uuid4().hex}@labnet.test>", "",
                        "final alpha.113 acceptance test mail over 465", ""])
    refused5 = s5.sendmail(frm, [to], msg5)
    out["smtp_465_send"] = {"ok": not refused5, "refused": refused5}
    s5.quit()
except Exception as e:
    out["smtp_465_send"] = {"ok": False, "error": repr(e)}
out["subjects"] = [subject, subject465]
found = None
for i in range(int(__import__('os').environ.get('IMAP_TRIES','12'))):
    try:
        m = imaplib.IMAP4_SSL(host, 993, ssl_context=ctx, timeout=30)
        out["imaps_993_cert"] = names(m.sock.getpeercert())
        m.login(to, pw["to"])
        typ, sel = m.select("INBOX", readonly=True)
        if typ != "OK":
            out["imap_select"] = f"{typ} {sel}"
            m.logout(); time.sleep(3); continue
        typ, data = m.search(None, "SUBJECT", f'"{subject}"')
        ids = data[0].split()
        if ids:
            typ, hdr = m.fetch(ids[-1], "(BODY.PEEK[HEADER.FIELDS (SUBJECT FROM TO)])")
            found = {"after_seconds": round(time.time() - t0, 1), "headers": hdr[0][1].decode(errors="replace").strip()}
        m.logout()
    except Exception as e:
        out["imap_error"] = repr(e)
    if found: break
    time.sleep(3)
out["imap_read"] = found or {"found": False}
out["served_certificates"] = {f"{port}/{sni or 'no-sni'}": peek(port, sni or None) for port in (587, 993, 465) for sni in snis + [""]}
print(json.dumps(out, indent=1))
