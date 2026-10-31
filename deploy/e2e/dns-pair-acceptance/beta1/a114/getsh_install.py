#!/usr/bin/env python3
"""Install CelikPanel on a beta1 lab guest the way the owner does: in a root
terminal, the portal's command
    curl -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh &&
    sh celikpanel-install.sh
answering the first-administrator prompts on the terminal (password hidden).
Lab differences (documented in the report): celikpanel.net is the lab portal
copy served by a curl stand-in on the session's PATH (the entry-boundary
contract's seam), and get.sh's pinned release-key digest is the lab key's.
usage: getsh_install.py TOOLS_BETA1_DIR CELL NODE   (root, archlinux WSL host). Lab only.
"""
import io, json, os, re, secrets, select, subprocess, sys, tarfile, time
from pathlib import Path

L = Path(sys.argv[1]); cell, node = sys.argv[2], sys.argv[3]
sys.path.insert(0, str(L)); sys.path.insert(0, str(L.parent))
import beta1, lab, install_steps  # noqa: E402

PORTAL = Path(os.environ["A114_PORTAL"])
SESSION = """# owner terminal session (lab): only PATH differs, see the curl stand-in
cd /root
export PATH=/root/lab-portal/bin:$PATH
curl -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh &&
sh celikpanel-install.sh
"""

st = beta1.load_state(cell, node)
if not st or st.get("password"):
    raise SystemExit("node not provisioned or already installed")
password = secrets.token_urlsafe(24)
st["password"], st["username"] = password, "owner"
beta1.save_state(cell, node, st)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
out = {"method": "get.sh in a root terminal (ssh -tt), lab portal", "session": SESSION}
t0 = time.monotonic()

# stage the lab portal copy + curl stand-in + the session file on the guest
buf = io.BytesIO()
with tarfile.open(fileobj=buf, mode="w") as t:
    t.add(str(PORTAL / "site"), arcname="lab-portal/site")
    t.add(str(PORTAL / "curl"), arcname="lab-portal/bin/curl")
    data = SESSION.encode(); ti = tarfile.TarInfo("lab-portal/owner-session.sh"); ti.size = len(data); ti.mode = 0o644
    t.addfile(ti, io.BytesIO(data))
s = beta1.guest(cell, node, "sudo -n sh -c 'umask 022; tar --no-same-owner -xf - -C /root && chown -R root:root /root/lab-portal && chmod 755 /root/lab-portal/bin/curl && test ! -e /root/celikpanel-install.sh && echo staged'",
                stdin=buf.getvalue(), timeout=300)
out["stage"] = {"rc": s.returncode, "out": s.stdout.decode()[-300:], "err": s.stderr.decode()[-300:]}
if s.returncode:
    print(json.dumps(out, indent=1)); sys.exit(1)

argv = lab.ssh_argv(cell, node, "sudo -n bash /root/lab-portal/owner-session.sh")
argv.insert(1, "-tt")
p = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                     env={**os.environ, "PATH": beta1.PATH_ENV})
answers = [("Admin username", "owner"), ("Admin email", "owner@beta1-lab.test"),
           ("Show password while typing", ""), ("Admin password", password), ("Confirm password", password)]
raw = b""; pos = 0; sent = []; deadline = time.monotonic() + 3600; bad = None
while True:
    if time.monotonic() > deadline:
        bad = "timeout"; p.kill(); break
    r, _, _ = select.select([p.stdout], [], [], 5)
    if r:
        chunk = os.read(p.stdout.fileno(), 65536)
        if not chunk:
            break
        raw += chunk
        text = raw.decode("utf-8", "replace")
        if answers:
            i = text.find(answers[0][0], pos)
            if i >= 0:
                prompt, reply = answers.pop(0)
                time.sleep(0.5)
                p.stdin.write(reply.encode() + b"\n"); p.stdin.flush()
                sent.append({"prompt": prompt, "at_seconds": round(time.monotonic() - t0, 1)})
                pos = i + len(prompt)
        if re.search(r"Invalid username|must not be empty|do not match|at least 8|Please enter y/e", text[pos:]):
            bad = "prompt refused an answer"; p.kill(); break
rc = p.wait()
text = install_steps.ANSI_RE.sub("", raw.decode("utf-8", "replace")).replace("\r", "")
text = rec.redactor.text(text)
if password in text:
    text = text.replace(password, "[REDACTED]")
(beta1.EVID / cell).mkdir(parents=True, exist_ok=True)
(beta1.EVID / cell / f"getsh-terminal-{node}.txt").write_text(text)
out["terminal"] = {"rc": rc, "seconds": round(time.monotonic() - t0, 1), "answered": sent, "problem": bad,
                   "tail": text[-3000:], "restart_required": install_steps.RESTART_REQUIRED_BANNER in text,
                   "restart_recommended": install_steps.RESTART_RECOMMENDED_BANNER in text}
g = beta1.guest(cell, node, "sudo -n cat /root/lab-portal/requests.log; sudo -n ls -l /root/celikpanel-install.sh; sudo -n sha256sum /root/celikpanel-install.sh", timeout=60)
out["portal_requests"] = g.stdout.decode()
pc = beta1.guest(cell, node, install_steps.postinstall_check(), timeout=60)
out["postinstall"] = {"rc": pc.returncode, "out": pc.stdout.decode()[-600:]}
if out["terminal"]["restart_required"]:
    out["owner_restart"] = lab.reboot(cell, node, 900)
    time.sleep(10)
    p2 = beta1.guest(cell, node, "systemctl is-active celikpanel-agent.service celikpanel-panel.service", timeout=60)
    out["after_restart_units"] = p2.stdout.decode()
out["seconds"] = round(time.monotonic() - t0, 1)
rec.write(f"{node}-install", out)
print(json.dumps(out, indent=1))
sys.exit(0 if rc == 0 and pc.returncode == 0 else 1)
