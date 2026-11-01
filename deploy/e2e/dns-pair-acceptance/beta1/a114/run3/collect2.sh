#!/bin/bash
# Collect the redacted evidence of the alpha.114 final lab acceptance run 2 into the lab worktree.
# usage: collect114.sh "CELL:NODE ..."  (journals of the guests still running). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/final-alpha114-run2-20261101"
for c in $1; do
  CELL=${c%%:*}; n=${c##*:}
  b1 sh $CELL $n "journalctl -u 'celikpanel*' -u pdns -u nginx -u postfix -u 'postfix@*' -u dovecot -u rspamd --no-pager -o short-iso -b all | grep -vE 'metrics' | tail -n 1500" --label journal-$n --limit 400000 > /dev/null
  b1 sh $CELL $n "cat /opt/celikpanel/release.commit; grep PRETTY_NAME /etc/os-release; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; nginx -t 2>&1 | tail -1; systemctl is-active celikpanel-panel celikpanel-agent pdns nginx postfix dovecot | paste -sd' '; systemctl --failed --no-legend | head; journalctl -b -p err --no-pager -o short-iso | grep -v kernel | tail -15" --label versions-$n --limit 12000 > /dev/null
  echo "collected $c"
done
T=/var/tmp/a114r3/evidence-stage
rm -rf $T; mkdir -p $T/logs $T/raw $T/owner-log $T/drivers $T/portal
cp /var/tmp/a114r3/logs/*.log /var/tmp/a114r3/*.out $T/logs/ 2>/dev/null; mkdir -p $T/logs/e1; cp /var/tmp/a114r3/e1/op-*.json $T/logs/e1/ 2>/dev/null; cp /var/tmp/a114r3/out-build-114.out /var/tmp/a114r3/budget-*.log $T/logs/ 2>/dev/null
for d in /var/tmp/cp-beta1/evidence/a4-*; do [ -d $d ] && cp -r $d $T/raw/; done
find $T -name .seq -delete
cp /var/tmp/a114r3/owner-log/* $T/owner-log/ 2>/dev/null
cp /var/tmp/a114r3/portal-v0.1.0-alpha.114/get.sh.diff $T/portal/get.sh.diff
cp /var/tmp/a114r3/portal-v0.1.0-alpha.114/site/releases/*/linux/amd64/release-manifest-v2 $T/portal/ 2>/dev/null
cp /var/tmp/a114r3/portal-key/public.pem $T/portal/lab-release-signing-public-key.pem
cp $H/*.sh $H/*.py $H/*.js $T/drivers/ 2>/dev/null
cp -r /var/tmp/a114r3/shots-webp $T/shots; cp -r /var/tmp/a114r3/attempt1 $T/raw/attempt1-before-host-reboot; mkdir -p $T/logs/e1-forced; cp /var/tmp/a114r3/e1e/op-r*.json $T/logs/e1-forced/ 2>/dev/null; cp /var/tmp/a114r3/coll-*.out $T/logs/ 2>/dev/null
for v in b6800f082b5e277e4b63a38a6447af1ddf57359c d3f3a323da46ce8f8dca5ad4385666c5428c353e; do
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps({k:d.get(k) for k in ("commit","tree","version","sha256","license_mode","release","root","panel_build_tags")}))' /var/tmp/cp-pair-accept/dist/$v-acceptance-license/dist.json
done > $T/logs/dist.json
python3 -I $H/sanitize.py $T
# secrets: harness state (passwords, cookies, join codes) + owner-secrets (users, mailboxes, protect) + lab keys
python3 - $T <<'PY' > $T/logs/secret-scan.json
import json, pathlib, re, sys
root = pathlib.Path(sys.argv[1]); secrets = set()
for st in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*.json"):
    try: d = json.loads(st.read_text())
    except Exception: continue
    def walk(x):
        if isinstance(x, str) and len(x) >= 8: secrets.add(x)
        elif isinstance(x, dict): [walk(v) for k, v in x.items() if k not in ("os", "ip", "provisioned_at", "username", "pin", "panel_port", "plan_id", "package_updates_digest", "start_request_id", "ids")]
        elif isinstance(x, list): [walk(v) for v in x]
    if st.name == "owner-secrets.json" or st.name.startswith("mbx-"):
        walk(d)
    else:
        for k in ("password", "cookie"):
            if d.get(k): secrets.add(d[k])
        for c in d.get("join_codes", []): secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*last-join-code"):
    c = f.read_text().strip(); secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("invite-link-path"):
    secrets.add(f.read_text().strip().split("#t=")[-1])
for k in ("/var/tmp/a114r3/portal-key/private.pem", "/var/tmp/a114r3/portalnet/ca-key.pem", "/var/tmp/a114r3/portalnet/tls-key.pem"):
    p = pathlib.Path(k)
    if p.exists(): secrets.add(p.read_text().strip().splitlines()[1])
pem = re.compile(r"BEGIN [A-Z ]*PRIVATE KEY"); code = re.compile(r"\b[A-Z2-7]{4}(?:-[A-Z2-7]{4}){5}\b")
tok = re.compile(r"#t=(?!\[REDACTED\])[A-Za-z0-9_-]{20,}")
cred = re.compile(r'"(link_credential|enrollment_code|reverse_credential|peer_credential|proof|secret|recovery_codes|pending_token)"\s*:\s*"(?!\[REDACTED\])[^"]+')
pwv = re.compile(r'''['"](FTPPassword|password|new_password)['"][ ]*:[ ]*['"](?!\[REDACTED)[^'"]+''')
bad, n = [], 0
for p in root.rglob("*"):
    if not p.is_file() or p.suffix == ".png": continue
    n += 1; t = p.read_text(errors="replace")
    for s in secrets:
        if s and s in t: bad.append((str(p), "registered secret")); break
    for name, rx in (("pem private key", pem), ("join-code shape", code), ("link token", tok), ("credential value", cred), ("password value", pwv)):
        if rx.search(t): bad.append((str(p), name))
print(json.dumps({"files": n, "secrets_checked": len(secrets), "findings": bad[:30], "clean": not bad}))
PY
cat $T/logs/secret-scan.json
grep -q '"clean": true' $T/logs/secret-scan.json || { echo "SCAN NOT CLEAN - nothing copied"; exit 1; }
mkdir -p "$E"
( cd $T && tar --exclude=raw/attempt1-before-host-reboot/owner-log -czf "$E/final-alpha114-run2-evidence.tar.gz" raw logs owner-log portal drivers )
( cd "$E" && sha256sum final-alpha114-run2-evidence.tar.gz > final-alpha114-run2-evidence.tar.gz.sha256 )
( cd $T && tar -czf "$E/final-alpha114-run2-screenshots.tar.gz" shots )
( cd "$E" && sha256sum final-alpha114-run2-screenshots.tar.gz > final-alpha114-run2-screenshots.tar.gz.sha256 )
cp $T/logs/secret-scan.json $T/logs/dist.json $T/portal/get.sh.diff "$E/"
for f in pairA.out archB.out opsA.out opsB.out opsB3.out pairR.out pairR3.out opsA3.out pairF.out cd.out pairC.out pairD.out; do [ -f /var/tmp/a114r3/$f ] && cp /var/tmp/a114r3/$f "$E/"; done
ls -la "$E"; du -sh "$E"
