# guest (root, read-only): the served web bundle (every JS file under the installed web root, the index checked
# against what the panel serves) searched for native browser dialogs. Lab only.
I=$(ls -d /opt/celikpanel/web/index.html /opt/celikpanel/web/dist/index.html /opt/celikpanel/current/web/dist/index.html 2>/dev/null | head -1)
[ -z "$I" ] && I=$(find /opt/celikpanel -maxdepth 4 -name index.html -path '*web*' | head -1)
W=$(dirname $I); P=$(sed -n 's/^CELIKPANEL_LISTEN=.*://p' /etc/celikpanel/panel.env)
echo "web root $W ($(find $W -name '*.js' | wc -l) js files, $(du -sh $W | cut -f1)); panel port $P"
curl -sk https://127.0.0.1:$P/ -o /tmp/idx.html; echo "served index == installed index: $(cmp -s /tmp/idx.html $I && echo yes || echo NO)"
n=0; bad=0; for a in $(grep -oE '/assets/[A-Za-z0-9._-]+\.js' /tmp/idx.html | sort -u); do n=$((n+1)); curl -sk https://127.0.0.1:$P$a | cmp -s - $W$a || bad=$((bad+1)); done; echo "index scripts served == installed: $n checked, $bad differ"
python3 - "$W" <<'P'
import os, re, sys
pat = re.compile(r'(window|globalThis|self|top|parent)\.(confirm|alert|prompt)\b|(?<![\w$.])(confirm|alert|prompt)\(')
tot = 0
for root, _, files in os.walk(sys.argv[1]):
    for f in files:
        if not f.endswith(".js"): continue
        t = open(os.path.join(root, f), errors="replace").read()
        for m in pat.finditer(t):
            tot += 1
            print("  MATCH", f, repr(t[max(0, m.start() - 60): m.end() + 40]))
print("native dialog calls found:", tot)
P
echo "own confirm window present: $(grep -l 'confirm-action-confirm' $W/assets/*.js | wc -l) file(s); progress window: $(grep -l 'operation-progress' $W/assets/*.js | wc -l) file(s)"
