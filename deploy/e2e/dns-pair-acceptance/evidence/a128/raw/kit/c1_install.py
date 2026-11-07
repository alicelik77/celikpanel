#!/usr/bin/env python3
"""alpha.127 case 1: the owner's get.sh command on a fresh Debian 13 (lab portal through the curl stand-in).
A) a credentials file with a weak password: get.sh must refuse before downloading or changing anything (EN+TR rule);
B) the terminal prompt: a weak password first (refused, asked again, rule named), then a strong one; install completes.
usage: c1_install.py L CELL NODE   (root, archlinux WSL host). Lab only."""
import io, json, os, re, secrets, select, subprocess, sys, tarfile, time
from pathlib import Path
L = Path(sys.argv[1]); cell, node = sys.argv[2], sys.argv[3]
sys.path.insert(0, str(L)); sys.path.insert(0, str(L.parent))
import beta1, lab, install_steps  # noqa: E402
PORTAL = Path(os.environ["A114_PORTAL"])
CMD = "curl -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh &&\nsh celikpanel-install.sh\n"
SESSION_A = "cd /root\nexport PATH=/root/lab-portal/bin:$PATH\nexport CELIKPANEL_ADMIN_CREDENTIALS_FILE=/root/admin-weak.json\n" + CMD
SESSION_B = "cd /root\nexport PATH=/root/lab-portal/bin:$PATH\nrm -f celikpanel-install.sh\n" + CMD
SNAP_SH = """dpkg-query -W -f '${Package} ${Version}\\n' | sha256sum | cut -c1-16
ls -d /opt/celikpanel /etc/celikpanel /var/lib/celikpanel 2>&1 | tr '\\n' ' '; echo
systemctl list-unit-files 'celikpanel*' --no-legend | wc -l
ls /etc/apt/sources.list.d | tr '\\n' ' '; echo
echo requests:; cat /root/lab-portal/requests.log 2>/dev/null; echo end
"""
st = beta1.load_state(cell, node)
password = secrets.token_urlsafe(18) + "-Aa"
weak_file_pw = "weakpassword1"          # no uppercase, no special
weak_typed = "password1"                # no uppercase, no special
st["password"], st["username"] = password, "owner"; beta1.save_state(cell, node, st)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
t0 = time.monotonic()
buf = io.BytesIO()
with tarfile.open(fileobj=buf, mode="w") as t:
    t.add(str(PORTAL / "site"), arcname="lab-portal/site"); t.add(str(PORTAL / "curl"), arcname="lab-portal/bin/curl")
    for name, data in (("lab-portal/session-a.sh", SESSION_A), ("lab-portal/session-b.sh", SESSION_B), ("lab-portal/snap.sh", SNAP_SH)):
        d = data.encode(); ti = tarfile.TarInfo(name); ti.size = len(d); ti.mode = 0o644; t.addfile(ti, io.BytesIO(d))
s = beta1.guest(cell, node, "sudo -n sh -c 'umask 022; tar --no-same-owner -xf - -C /root && chown -R root:root /root/lab-portal && chmod 755 /root/lab-portal/bin/curl && echo staged'", stdin=buf.getvalue(), timeout=300)
print("stage", s.returncode, s.stdout.decode()[-100:])
cred = json.dumps({"username": "owner", "email": "owner@beta1-lab.test", "password": weak_file_pw}, separators=(",", ":")) + "\n"
beta1.guest(cell, node, "sudo -n sh -c 'umask 077; cat > /root/admin-weak.json'", stdin=cred.encode(), timeout=60)
SNAP = "sudo -n bash /root/lab-portal/snap.sh"
before = beta1.guest(cell, node, SNAP, timeout=60).stdout.decode()
print("== A: credentials file with a weak password\n-- before:\n" + before)
argv = lab.ssh_argv(cell, node, "sudo -n bash /root/lab-portal/session-a.sh")
r = subprocess.run(argv, capture_output=True, timeout=600, env={**os.environ, "PATH": beta1.PATH_ENV})
ta = install_steps.ANSI_RE.sub("", (r.stdout + r.stderr).decode("utf-8", "replace")).replace("\r", "")
print("-- exit", r.returncode, "\n-- terminal (tail):\n" + ta[-2500:])
after = beta1.guest(cell, node, SNAP, timeout=60).stdout.decode()
print("-- after:\n" + after)
print("-- packages, product dirs, units, apt sources unchanged:", before.split("\n")[:4] == after.split("\n")[:4])
beta1.guest(cell, node, "sudo -n sh -c 'rm -f /root/admin-weak.json /root/celikpanel-install.sh; : > /root/lab-portal/requests.log'", timeout=60)
print("\n== B: the terminal prompt, a weak password first")
argv = lab.ssh_argv(cell, node, "sudo -n bash /root/lab-portal/session-b.sh"); argv.insert(1, "-tt")
p = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env={**os.environ, "PATH": beta1.PATH_ENV})
answers = [("Admin username", "owner"), ("Admin email", "owner@beta1-lab.test"), ("Show password while typing", ""),
           ("Admin password", weak_typed), ("Please try again", None), ("Admin password", password), ("Confirm password", password)]
raw = b""; pos = 0; sent = []; deadline = time.monotonic() + 3600; bad = None; refusal_state = None
while True:
    if time.monotonic() > deadline: bad = "timeout"; p.kill(); break
    rr, _, _ = select.select([p.stdout], [], [], 5)
    if not rr: continue
    chunk = os.read(p.stdout.fileno(), 65536)
    if not chunk: break
    raw += chunk; text = raw.decode("utf-8", "replace")
    while answers:
        i = text.find(answers[0][0], pos)
        if i < 0: break
        prompt, reply = answers.pop(0); pos = i + len(prompt)
        if reply is None:
            refusal_state = beta1.guest(cell, node, "sudo -n sh -c 'ls /var/lib/celikpanel 2>&1 | head -8; systemctl is-active celikpanel-panel 2>&1'", timeout=60).stdout.decode()
            sent.append({"prompt": "refusal seen", "at": round(time.monotonic() - t0, 1)}); continue
        time.sleep(0.5); p.stdin.write(reply.encode() + b"\n"); p.stdin.flush()
        sent.append({"prompt": prompt, "at": round(time.monotonic() - t0, 1), "weak": reply == weak_typed})
rc = p.wait()
text = install_steps.ANSI_RE.sub("", raw.decode("utf-8", "replace")).replace("\r", "")
text = rec.redactor.text(text).replace(password, "[REDACTED]").replace(weak_typed, "[WEAK]")
(beta1.EVID / cell).mkdir(parents=True, exist_ok=True); (beta1.EVID / cell / f"c1-terminal-{node}.txt").write_text("== A\n" + ta + "\n== B\n" + text)
k = text.find("Admin username")
print("-- prompt section:\n" + text[max(0, k - 600): k + 1800])
print("-- server state when the weak password was refused:\n", refusal_state)
print("-- answered", json.dumps(sent), "rc", rc, "problem", bad)
print("-- tail:\n" + text[-2000:])
pc = beta1.guest(cell, node, install_steps.postinstall_check(), timeout=60)
print("-- postinstall rc", pc.returncode, pc.stdout.decode()[-600:])
if install_steps.RESTART_REQUIRED_BANNER in text:
    print("restart", lab.reboot(cell, node, 900)); time.sleep(10)
u = beta1.guest(cell, node, "systemctl is-active celikpanel-agent.service celikpanel-panel.service; systemctl --failed --no-legend | cat", timeout=60)
print("-- units", u.stdout.decode())
print("seconds", round(time.monotonic() - t0, 1))
