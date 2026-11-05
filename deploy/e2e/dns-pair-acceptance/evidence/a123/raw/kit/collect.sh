# Collect alpha.123 round evidence into the lab114 worktree (scrubbed). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a123"
rm -rf "$E"; mkdir -p "$E/raw/kit"
for f in $LOGS/*; do s=$(stat -c %s "$f"); [ "$s" -lt 400000 ] && cp "$f" "$E/raw/"; [ "$s" -ge 400000 ] && echo "skipped large $f $s"; done
cp $B/portal-v0.1.0-alpha.123/get.sh.diff "$E/raw/get.sh.diff-9b69021a9" 2>/dev/null
cp /var/tmp/cp-pair-accept/dist/$C118-acceptance-license/dist.json "$E/raw/dist-9b69021a9.json" 2>/dev/null
cp $B/portal-v0.1.0-alpha.123/site/releases/v0.1.0-alpha.123/linux/amd64/release-manifest-v2 "$E/raw/release-manifest-v2-9b69021a9" 2>/dev/null
for f in $A/*.out; do [ -e "$f" ] || continue; cp "$f" "$E/raw/run-$(basename $f .out).txt"; done
cp $A/*.sh $A/*.py "$E/raw/kit/"
python3 - "$E" $B <<'P'
import os, re, sys, json, glob
secrets = []
for f in glob.glob(sys.argv[2] + "/t-*/cred-*.json"):
    try: secrets.append(json.load(open(f))["password"])
    except Exception: pass
for st in glob.glob("/var/tmp/cp-beta1/state/*/*.json"):
    try:
        d = json.load(open(st))
        for k in ("password", "cookie"):
            if isinstance(d.get(k), str) and len(d[k]) > 6: secrets.append(d[k])
    except Exception: pass
for f in glob.glob(sys.argv[2] + "/t-*/*.cookie"):
    try: secrets.append(open(f).read().strip())
    except Exception: pass
secrets = [s for s in secrets if s and len(s) > 6]
n = 0
for root, _, files in os.walk(sys.argv[1]):
    for fn in files:
        p = os.path.join(root, fn); t = open(p, errors="replace").read(); o = t
        for s in secrets: t = t.replace(s, "[REDACTED]")
        t = re.sub(r'("(?:FTPPassword|admin_password|owner_password|password|passphrase|private_key|cookie)"\s*:\s*")[^"]+(")', r'\1[REDACTED]\2', t)
        if t != o: open(p, "w").write(t); n += 1
print("scrubbed files:", n, "known secrets:", len(secrets))
pat = re.compile(r'(password"?\s*[:=]\s*"?(?!\[REDACTED\])[A-Za-z0-9_\-]{8,}|BEGIN [A-Z ]*PRIVATE|SSHPASS=\S+|celikpanel_session=[A-Za-z0-9_\-]{8,})', re.I)
for root, _, files in os.walk(sys.argv[1]):
    for fn in files:
        for i, line in enumerate(open(os.path.join(root, fn), errors="replace"), 1):
            for m in pat.finditer(line): print("CHECK", fn, i, line[max(0, m.start()-40): m.end()+20].strip()[:160])
P
du -sh "$E"; ls "$E/raw" | grep -v kit | tr "\n" " "; echo
