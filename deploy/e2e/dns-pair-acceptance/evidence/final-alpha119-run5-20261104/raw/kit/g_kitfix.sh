# guest (root): LAB KIT FIXES in /root/kit (not product code), recorded as a diff:
#  1. mark() returned 1 after a PASS without detail, so every "cond && mark N PASS || mark N FAIL msg" also marked FAIL.
#  2. item 18 ran `nsenter -m test` inside the program's mount namespace, where exec is denied (NoExecPaths) -> read
#     the program's view through /proc/<pid>/root instead.
cd /root/kit/deploy/e2e/sandbox-escape; cp run.sh run.sh.orig
python3 - <<'P'
t=open("run.sh").read()
t=t.replace('	[ -n "$detail" ] && DETAIL[$item]="${DETAIL[$item]:-}${DETAIL[$item]:+; }$detail"\n}','	[ -n "$detail" ] && DETAIL[$item]="${DETAIL[$item]:-}${DETAIL[$item]:+; }$detail"\n	return 0\n}',1)
t=t.replace('if nsenter -t "$pid" -m test -x "$APP/bin/goprobe" && ! nsenter -t "$pid" -m test -e "$APP/secret.txt"; then','if [ -x "/proc/$pid/root$APP/bin/goprobe" ] && [ ! -e "/proc/$pid/root$APP/secret.txt" ]; then',1)
open("run.sh","w").write(t)
P
diff run.sh.orig run.sh
