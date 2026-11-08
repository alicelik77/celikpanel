# Collect the alpha.130 round evidence into the lab114 worktree (scrubbed). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a130"
V=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/view
rm -rf "$E"; mkdir -p "$E/raw/kit" "$E/shots"
for f in $LOGS/*; do s=$(stat -c %s "$f"); [ "$s" -lt 600000 ] && cp "$f" "$E/raw/"; [ "$s" -ge 600000 ] && echo "skipped large $f $s"; done
cp /var/tmp/cp-beta1/evidence/s0/getsh-terminal-*.txt "$E/raw/" 2>/dev/null
for v in 129 130 131 132; do cp $B/portal-v0.1.0-alpha.$v/site/releases/v0.1.0-alpha.$v/linux/amd64/release-manifest-v2 "$E/raw/release-manifest-v2-$v" 2>/dev/null; done
for f in $B/out-build*.out; do cp $f "$E/raw/$(basename $f .out).txt"; done
for f in $A/pairP.out $A/flushP.out; do [ -e "$f" ] && cp "$f" "$E/raw/run-$(basename $f .out).txt"; done
for f in $A/../*.out; do cp "$f" "$E/raw/run-$(basename $f .out).txt"; done
for f in $A/*.sh $A/*.py $A/*.js $A/*.json; do cp $f "$E/raw/kit/"; done
for f in $V/*-tr-desktop.png $V/A2-*.png $V/A4-*.png; do [ -e "$f" ] && cp "$f" "$E/shots/"; done
[ -f $A/README-a130.md ] && cp $A/README-a130.md "$E/README.md"
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
for f in glob.glob(sys.argv[2] + "/t-*/*.cookie") + glob.glob(sys.argv[2] + "/t-*/tok*.txt"):
    try: secrets.append(open(f).read().strip())
    except Exception: pass
secrets = [s for s in secrets if s and len(s) > 6]
n = 0
for root, _, files in os.walk(sys.argv[1]):
    for fn in files:
        p = os.path.join(root, fn)
        if fn.endswith(".png"): continue
        t = open(p, errors="replace").read(); o = t
        for s in secrets: t = t.replace(s, "[REDACTED]")
        if "/kit" not in root:
            t = re.sub(r'("(?:FTPPassword|admin_password|owner_password|password|passphrase|private_key|cookie|token|current_password|new_password)"\s*:\s*")[^"]+(")', r'\1[REDACTED]\2', t)
            t = re.sub(r'#t=[A-Za-z0-9_.~-]{8,}', '#t=[REDACTED]', t)
        if t != o: open(p, "w").write(t); n += 1
print("scrubbed files:", n, "known secrets:", len(secrets))
pat = re.compile(r'(password"?\s*[:=]\s*"?(?!\[REDACTED\])[A-Za-z0-9_\-]{12,}|BEGIN [A-Z ]*PRIVATE|cpk_[A-Za-z0-9]{12}_[A-Za-z0-9_-]{20,}|celikpanel_session=[A-Za-z0-9_\-]{8,}|#t=[A-Za-z0-9_-]{20,})', re.I)
for root, _, files in os.walk(sys.argv[1]):
    for fn in files:
        if fn.endswith(".png"): continue
        for i, line in enumerate(open(os.path.join(root, fn), errors="replace"), 1):
            for m in pat.finditer(line): print("CHECK", fn, i, line[max(0, m.start()-40): m.end()+20].strip()[:160])
P
du -sh "$E"; ls "$E/raw" | wc -l
