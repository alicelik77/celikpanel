#!/bin/bash
# D-039 real-system run: a website certificate issued by the Agent's own
# code keeps renewing and being served by the operating system's certbot
# with no CelikPanel program on the system, and the Agent reads what
# happened. Everything runs in the disposable chroot built by
# build-chroot.sh, inside its own network namespace, against a local Pebble
# test CA (Debian's pebble package). Never run on an installed server.
#
# Usage (root, development machine): run.sh <chroot-dir> <source-dir> <out-dir>
#
# D-039 gerçek sistem denemesi: Agent'ın verdiği sertifikayı, sistemde hiçbir
# CelikPanel programı yokken işletim sisteminin certbot'u yeniler ve sunar.
set -euo pipefail
root=${1:?chroot directory required}
src=${2:?source directory required}
out=${3:?evidence directory required}
case "$root" in /var/tmp/cp-d039-*) ;; *) echo "refusing chroot $root" >&2; exit 2 ;; esac
[ -f "$root/etc/celikpanel-sitecert-chroot" ] || { echo "not the disposable chroot" >&2; exit 2; }
ns=cp-d039
bin=/var/tmp/cp-d039-agent.test
mkdir -p "$out"
log() { printf '== %s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$out/run.log"; }
fail() { log "FAIL: $*"; exit 1; }

export PATH=/opt/celikpanel-s2/go/bin:$PATH GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOCACHE=/var/tmp/sitecert-gocache
log "build the Agent's realsystem test binary from $src"
(cd "$src" && go test -c -tags realsystem -o "$bin" ./cmd/agent)
sha256sum "$bin" | tee -a "$out/run.log"
(cd "$src" && git rev-parse HEAD 2>/dev/null || true) | tee -a "$out/run.log"

cleanup() {
  ip netns exec "$ns" chroot "$root" /bin/sh -c 'pkill -x nginx; pkill -x pebble' 2>/dev/null || true
  rm -f "$root/root/agent.test"
  umount -l "$root/proc" 2>/dev/null || true
  umount -l "$root/dev" 2>/dev/null || true
  ip netns del "$ns" 2>/dev/null || true
}
trap cleanup EXIT
ip netns add "$ns"
ip netns exec "$ns" ip link set lo up
mount -t proc proc "$root/proc"
mount --rbind /dev "$root/dev"
mount --make-rslave "$root/dev"

# Commands inside the chroot get a clean environment: no CelikPanel path, no
# CA override; certbot trusts Pebble only through the chroot's own trust store.
inchroot() {
  ip netns exec "$ns" chroot "$root" /usr/bin/env -i PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin HOME=/root LANG=C.UTF-8 "$@"
}
phase() {
  local name=$1; shift
  log "phase $name (Agent code present)"
  install -m 0700 "$bin" "$root/root/agent.test"
  if ! inchroot "$@" /root/agent.test -test.run "^${name}\$" -test.v -test.count=1 >>"$out/phase-$name.log" 2>&1; then
    rm -f "$root/root/agent.test"
    tail -30 "$out/phase-$name.log"
    fail "phase $name"
  fi
  tail -5 "$out/phase-$name.log" | tee -a "$out/run.log"
  rm -f "$root/root/agent.test"
}
served() {
  inchroot /bin/sh -c 'openssl s_client -connect 127.0.0.1:443 -servername site.test </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256 -enddate'
}
live() {
  inchroot /bin/sh -c "openssl x509 -in /etc/letsencrypt/live/$(cat "$root/root/sitecert.lineage")/cert.pem -noout -fingerprint -sha256 -enddate"
}
no_celikpanel_program() {
  # Nothing of CelikPanel that could run: no test binary, no Agent, no Panel.
  if [ -e "$root/root/agent.test" ] || [ -e "$root/opt/celikpanel" ] || [ -e "$root/usr/local/bin/celikpanel-agent" ]; then
    fail "a CelikPanel program is on the system"
  fi
  # The chroot shares the development host's process table: count only
  # processes whose root directory is this chroot.
  local pid found=""
  for pid in $(ls /proc | grep -E '^[0-9]+$'); do
    [ "$(readlink "/proc/$pid/root" 2>/dev/null)" = "$root" ] || continue
    if tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null | grep -qiE 'celikpanel|agent\.test|/agent( |$)|/panel( |$)'; then
      found="$found $pid"
    fi
  done
  [ -z "$found" ] || fail "a CelikPanel process runs in the chroot:$found"
  log "no CelikPanel program on the system (no binary, no process rooted in the chroot)"
}
# The operating system's own renewal: the exact ExecStart of the chroot's
# certbot.service (what certbot.timer starts on Debian).
os_renew() {
  local exec
  exec=$(sed -n 's/^ExecStart=//p' "$root/usr/lib/systemd/system/certbot.service")
  log "certbot.service ExecStart: $exec"
  set +e
  inchroot /bin/sh -c "$exec" >>"$out/os-renew-$1.log" 2>&1
  local rc=$?
  set -e
  log "certbot renew exit $rc"
  return $rc
}

log "Pebble test CA (local only), EAB required"
# Debian's pebble package ships no TLS material for its own API: a throwaway
# CA for https://localhost:14000, trusted only inside this chroot.
inchroot /bin/sh -ec '
  mkdir -p /root/pebble-tls && cd /root/pebble-tls
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 -subj "/CN=D-039 throwaway API CA" -keyout ca.key -out ca.pem 2>/dev/null
  openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=localhost" -keyout key.pem -out req.pem 2>/dev/null
  printf "subjectAltName=DNS:localhost,IP:127.0.0.1\n" >ext.cnf
  openssl x509 -req -in req.pem -CA ca.pem -CAkey ca.key -CAcreateserial -days 2 -extfile ext.cnf -out cert.pem 2>/dev/null
  cp ca.pem /usr/local/share/ca-certificates/d039-pebble-api.crt
  update-ca-certificates >/dev/null'
hmac=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')
cat >"$root/root/pebble.json" <<EOF
{"pebble": {
  "listenAddress": "127.0.0.1:14000",
  "managementListenAddress": "127.0.0.1:15000",
  "certificate": "/root/pebble-tls/cert.pem",
  "privateKey": "/root/pebble-tls/key.pem",
  "httpPort": 80, "tlsPort": 5001, "ocspResponderURL": "",
  "externalAccountBindingRequired": true,
  "externalAccountMACKeys": {"kid-d039": "$hmac"},
  "certificateValidityPeriod": 86400
}}
EOF
inchroot /bin/sh -c 'PEBBLE_VA_NOSLEEP=1 PEBBLE_AUTHZREUSE=0 setsid pebble -config /root/pebble.json >/root/pebble.log 2>&1 </dev/null &'
for _ in $(seq 1 50); do inchroot curl -fsS https://localhost:14000/dir >/dev/null 2>&1 && break; sleep 0.2; done
inchroot curl -fsS https://localhost:14000/dir >>"$out/run.log" || fail "pebble did not start"

# systemctl stand-in (a chroot has no systemd): answers what the deploy hook
# and the scheduler reader ask, from the chroot's own files and processes.
cat >"$root/usr/local/sbin/systemctl" <<'EOF'
#!/bin/sh
echo "systemctl $*" >>/root/systemctl.calls
case "$1 $2" in
  "is-active nginx"|"is-active --quiet") pgrep -x nginx >/dev/null && { [ "$2" = --quiet ] || echo active; exit 0; }; echo inactive; exit 3 ;;
  "reload nginx") exec nginx -s reload ;;
  "is-enabled certbot.timer") [ -e /etc/systemd/system/timers.target.wants/certbot.timer ] && { echo enabled; exit 0; }; echo disabled; exit 1 ;;
  "is-active certbot.timer") [ -e /etc/systemd/system/timers.target.wants/certbot.timer ] && { echo active; exit 0; }; echo inactive; exit 3 ;;
  "is-enabled "*|"is-active "*) echo not-found; exit 4 ;;
  "enable --now") ln -sf /usr/lib/systemd/system/certbot.timer /etc/systemd/system/timers.target.wants/certbot.timer; exit 0 ;;
esac
exit 0
EOF
chmod 0755 "$root/usr/local/sbin/systemctl"
ls -l "$root/etc/systemd/system/timers.target.wants/" 2>&1 | tee -a "$out/run.log" || true

export_env=(SITECERT_ACME_SERVER=https://localhost:14000/dir SITECERT_EAB_KID=kid-d039 "SITECERT_EAB_HMAC=$hmac" CELIKPANEL_REAL_SYSTEM_SITECERT=1)
phase TestRealSystemSiteCertPrepare CELIKPANEL_REAL_SYSTEM_SITECERT=1
inchroot nginx -t >>"$out/run.log" 2>&1
inchroot nginx
phase TestRealSystemSiteCertIssue "${export_env[@]}"
inchroot nginx -t >>"$out/run.log" 2>&1
inchroot nginx -s reload
sleep 1
lineage=$(cat "$root/root/sitecert.lineage")
first_notafter=$(cat "$root/root/sitecert.notafter")
log "lineage $lineage, first certificate until $first_notafter"
s1=$(served); l1=$(live); log "served: $s1"; log "live:   $l1"
[ "$s1" = "$l1" ] || fail "nginx does not serve certbot's live certificate"
cp "$root/etc/letsencrypt/renewal/$lineage.conf" "$out/renewal.conf"
cp "$root/etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates" "$out/deploy-hook"
stat -c '%a %U:%G %n' "$root/etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates" | tee -a "$out/run.log"
cp "$root/etc/nginx/sites-available/site.test.conf" "$out/vhost.conf"
cp "$root/etc/nginx/celikpanel-managed.d/site.test/acme-http-01.conf" "$out/acme-http-01.conf"
if grep -rqF -- "$hmac" "$root/etc/letsencrypt" "$root/var/log/letsencrypt"; then fail "the EAB HMAC key reached certbot's files or logs"; fi
log "EAB HMAC key: not in /etc/letsencrypt, not in /var/log/letsencrypt"
stat -c '%a %n' "$root/var/log/letsencrypt" "$root/etc/letsencrypt/live" "$root/etc/letsencrypt/archive" | tee -a "$out/run.log"

log "--- 1. certbot's own dry run with the written configuration, no CelikPanel program"
no_celikpanel_program
# A dry run goes to the Let's Encrypt staging server unless --server is
# given; the netns has no route out, so it is pointed at Pebble explicitly.
inchroot certbot renew --dry-run --no-random-sleep-on-renew --server https://localhost:14000/dir --cert-name "$lineage" >>"$out/dry-run.log" 2>&1 || fail "dry run"
tail -3 "$out/dry-run.log" | tee -a "$out/run.log"

log "--- 2. the operating system renews it, no CelikPanel program"
no_celikpanel_program
os_renew ok || fail "certbot renew failed"
sleep 1
s2=$(served); l2=$(live); log "served: $s2"; log "live:   $l2"
[ "$s2" = "$l2" ] || fail "nginx does not serve the renewed certificate"
[ "$s2" != "$s1" ] || fail "the certificate was not renewed"
cat "$root/var/lib/celikpanel-site-certificates/$lineage" | tee -a "$out/run.log"
grep -qx 'nginx=reloaded' "$root/var/lib/celikpanel-site-certificates/$lineage" || fail "hook did not reload nginx"
phase TestRealSystemSiteCertObserve "${export_env[@]}" SITECERT_LABEL=renewed SITECERT_EXPECT_STATE=scheduled \
  "SITECERT_EXPECT_NEWER_THAN=$first_notafter" SITECERT_EXPECT_DEPLOY=reloaded

log "--- 3. a renewal that fails is read as failed with certbot's own reason"
mv "$root/etc/nginx/celikpanel-managed.d/site.test/acme-http-01.conf" "$root/root/acme-http-01.conf.saved"
inchroot nginx -s reload; sleep 1
no_celikpanel_program
if os_renew failed; then fail "renewal succeeded without the challenge location"; fi
grep -h "Failed to renew certificate $lineage" "$root/var/log/letsencrypt/letsencrypt.log" | tail -1 | tee -a "$out/run.log"
phase TestRealSystemSiteCertObserve "${export_env[@]}" SITECERT_LABEL=failed SITECERT_EXPECT_STATE=failed
mv "$root/root/acme-http-01.conf.saved" "$root/etc/nginx/celikpanel-managed.d/site.test/acme-http-01.conf"
inchroot nginx -s reload; sleep 1

log "--- 4. renewed, but nginx's configuration is broken: not reloaded, read as deploy_incomplete"
s3=$(served)
mkdir -p "$root/etc/nginx/celikpanel-sites.d/site.test"
echo "this_is_not_a_directive;" >"$root/etc/nginx/celikpanel-sites.d/site.test/broken.conf"
no_celikpanel_program
os_renew deploy || log "certbot renew reported the hook's failure (expected)"
grep -h "deploy-hook\|celikpanel-site-certificates" "$out/os-renew-deploy.log" | tail -2 | tee -a "$out/run.log" || true
cat "$root/var/lib/celikpanel-site-certificates/$lineage" | tee -a "$out/run.log"
grep -qx 'nginx=config_test_failed' "$root/var/lib/celikpanel-site-certificates/$lineage" || fail "hook outcome"
s4=$(served); l4=$(live)
[ "$s4" = "$s3" ] && [ "$l4" != "$s3" ] || fail "expected the previous certificate served and a newer one in live/"
log "served still the previous certificate; live/ has the renewed one"
phase TestRealSystemSiteCertObserve "${export_env[@]}" SITECERT_LABEL=deploy SITECERT_EXPECT_STATE=deploy_incomplete SITECERT_EXPECT_DEPLOY=config_test_failed
rm -f "$root/etc/nginx/celikpanel-sites.d/site.test/broken.conf"
inchroot nginx -s reload; sleep 1
s5=$(served); [ "$s5" = "$l4" ] || fail "after the owner's fix nginx serves the renewed certificate"
log "after the owner fixed nginx and reloaded it, the renewed certificate is served"

cp "$root"/root/sitecert-state-*.json "$out/" 2>/dev/null || true
cp "$root/root/systemctl.calls" "$out/" 2>/dev/null || true
cp "$root/root/pebble.log" "$out/" 2>/dev/null || true
cp "$root/root/package-versions.txt" "$out/" 2>/dev/null || true
log "PASS"
