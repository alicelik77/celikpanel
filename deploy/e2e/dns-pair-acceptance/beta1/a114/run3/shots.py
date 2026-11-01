#!/usr/bin/env python3
# usage: shots.py CELL NODE PREFIX ITEMS.json [--langs en,tr] [--widths desktop,phone] [--user owner] [--pwfile F] [--shot-login]
import json, sys, subprocess, argparse, pathlib, os
sys.path.insert(0, "/var/tmp/a114r3/h/deploy/e2e/dns-pair-acceptance/beta1"); sys.path.insert(0, "/var/tmp/a114r3/h/deploy/e2e/dns-pair-acceptance")
p = argparse.ArgumentParser(); p.add_argument("cell"); p.add_argument("node"); p.add_argument("prefix"); p.add_argument("items")
p.add_argument("--langs", default="en,tr"); p.add_argument("--widths", default="desktop,phone"); p.add_argument("--user", default="owner")
p.add_argument("--pwfile"); p.add_argument("--shot-login", action="store_true"); p.add_argument("--no-login", action="store_true")
a = p.parse_args()
import lab, beta1
ip = lab.address_of(a.cell, a.node)
st = beta1.load_state(a.cell, a.node)
port = st.get("panel_port") or beta1.panel_port(a.cell, a.node)
items = json.loads(pathlib.Path(a.items).read_text()) if a.items.endswith(".json") else json.loads(a.items)
for it in items:
    if str(it.get("path", "")).startswith("@file:"):
        it["path"] = pathlib.Path(it["path"][6:]).read_text().strip()
job = {"base": f"https://{ip}:{port}", "username": a.user, "password_file": a.pwfile or str(beta1.state_path(a.cell, a.node)),
       "outdir": "/var/tmp/a114r3/shots", "prefix": a.prefix, "langs": a.langs.split(","), "widths": a.widths.split(","),
       "items": items, "shotLogin": a.shot_login, "login": not a.no_login}
jf = pathlib.Path(f"/var/tmp/a114r3/shotjobs/{a.prefix}.json"); jf.parent.mkdir(parents=True, exist_ok=True); jf.write_text(json.dumps(job))
r = subprocess.run(["node", "/var/tmp/a114r3/shot.js", str(jf)], capture_output=True, text=True, timeout=1800)
jf.unlink()
print((r.stdout + r.stderr)[-6000:])
