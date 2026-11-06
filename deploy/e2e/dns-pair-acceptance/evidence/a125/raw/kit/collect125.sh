# Collect alpha.125 round evidence into the lab114 worktree (scrubbed). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a125"
rm -rf "$E"; mkdir -p "$E/raw/kit"
for f in $LOGS/*; do s=$(stat -c %s "$f"); [ "$s" -lt 400000 ] && cp "$f" "$E/raw/"; [ "$s" -ge 400000 ] && echo "skipped large $f $s"; done
cp $B/portal-v0.1.0-alpha.125/get.sh.diff "$E/raw/get.sh.diff-bf0a5b9ee" 2>/dev/null; cp $B/portal-v0.1.0-alpha.125/site/releases/v0.1.0-alpha.125/linux/amd64/release-manifest-v2 "$E/raw/release-manifest-v2-bf0a5b9ee"; cp $B/portal-v0.1.0-alpha.126/site/releases/v0.1.0-alpha.126/linux/amd64/release-manifest-v2 "$E/raw/release-manifest-v2-lab126" 2>/dev/null; mkdir -p "$E/raw/perf"; cp $B/perf/*.json "$E/raw/perf/" 2>/dev/null; for v in 124 125 126; do cp $B/out-build$v.out "$E/raw/build-$v.txt" 2>/dev/null; done 2>/dev/null
for f in $A/*.out; do case "$f" in *phase1.out|*build126.out) ;; esac; [ -e "$f" ] || continue; cp "$f" "$E/raw/run-$(basename $f .out).txt"; done
mkdir -p "$E/raw/run1"; cp $B/logs-run1/* "$E/raw/run1/" 2>/dev/null; for f in $A/run1/*.out; do cp "$f" "$E/raw/run1/run-$(basename $f .out).txt"; done
for f in env.sh lib.sh lib5.sh build.sh portal.sh portalF.sh phase1.sh sync.sh single124.sh pairX.sh build126.sh runP.sh runC.sh phaseR.sh retryM.sh pindns.sh pre.sh upd.sh post3.sh post4.sh case2.sh perf.py scanpoll.py gz.sh g_upd.sh g_mail5.sh g_mail587.sh g_maildir.sh g_cron.sh g_site.sh as.py samecheck.py repro1.sh g_pkgwatch.sh case2m.sh runC2.sh runC3.sh build2.sh build125c.sh extra5.sh d5evid.sh g_certdiag.sh g_isen.sh collect125.sh; do cp $A/$f "$E/raw/kit/" 2>/dev/null; done
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
