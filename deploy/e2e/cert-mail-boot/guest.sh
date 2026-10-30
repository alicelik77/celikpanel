#!/bin/sh
# Runs INSIDE a disposable QEMU guest booted from a distribution's cloud
# image (lab.sh). Never on an installed server: it refuses to run without the
# lab marker lab.sh writes through cloud-init.
#
# Stage "first": a local Pebble test CA, the Agent's own code (realsystem
# test binary) prepares the site, issues a native website certificate
# (D-040), secures mail.site.test with it (Postfix SNI map, Dovecot SNI
# drop-in in the release's own dialect), and reads the renewal state. Then
# the binary is removed; lab.sh reboots the guest.
# Stage "renew": with no CelikPanel program on the system, the operating
# system's own certbot timer fires (a runtime drop-in under /run moves only
# its schedule to once a minute), certbot renews the lineage, the site
# deploy hook reloads nginx, Postfix and Dovecot through the real systemctl,
# and every listener serves the new certificate. lab.sh reboots the guest.
# Stage "second": after the reboot the timer is still enabled and active and
# every listener still serves the renewed certificate. Stage "observe": the
# binary is copied back once to read the state.
#
# usage: guest.sh first|renew|second|observe
set -eu
stage=${1:?stage}
[ -f /etc/celikpanel-cert-mail-boot-lab ] || { echo "not the disposable lab guest" >&2; exit 2; }
out=/root/evidence
mkdir -p "$out"
log() { printf '== %s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$out/run.log"; }
fail() { log "FAIL: $*"; exit 1; }
. /etc/os-release
family=apt
command -v pacman >/dev/null 2>&1 && family=pacman

timer=certbot.timer
service=certbot.service
if [ "$family" = pacman ]; then timer=certbot-renew.timer; service=certbot-renew.service; fi

phase() { # name [VAR=value...]
  name=$1; shift
  log "phase $name (Agent code present)"
  [ -x /root/agent.test ] || fail "/root/agent.test is missing"
  if ! env -i PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin HOME=/root LANG=C.UTF-8 \
      CELIKPANEL_REAL_SYSTEM_SITECERT=1 SITECERT_ACME_SERVER=https://localhost:14000/dir "$@" \
      /root/agent.test -test.run "^${name}\$" -test.v -test.count=1 >"$out/phase-$name.log" 2>&1; then
    tail -30 "$out/phase-$name.log"
    fail "phase $name"
  fi
  grep -E -- '--- PASS|real_system_sitecert_test.go' "$out/phase-$name.log" | tail -4 | tee -a "$out/run.log"
}
fingerprint() { # port starttls|- sni
  if [ "$2" = - ]; then
    openssl s_client -connect "127.0.0.1:$1" -servername "$3" </dev/null 2>/dev/null
  else
    openssl s_client -connect "127.0.0.1:$1" -starttls "$2" -servername "$3" </dev/null 2>/dev/null
  fi | openssl x509 -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//'
}
live_fp() { openssl x509 -in "/etc/letsencrypt/live/$(cat /root/sitecert.lineage)/cert.pem" -noout -fingerprint -sha256 | sed 's/.*=//'; }
served_all() { # label: every listener serves the live certificate
  live=$(live_fp)
  https=$(fingerprint 443 - site.test)
  smtp=$(fingerprint 25 smtp mail.site.test)
  imaps=$(fingerprint 993 - mail.site.test)
  default_imaps=$(openssl s_client -connect 127.0.0.1:993 -noservername </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256 | sed 's/.*=//')
  log "$1: live=$live https=$https smtp(SNI mail.site.test)=$smtp imaps(SNI mail.site.test)=$imaps imaps(no SNI)=$default_imaps"
  [ "$https" = "$live" ] && [ "$smtp" = "$live" ] && [ "$imaps" = "$live" ] || fail "$1: not every listener serves the live certificate"
  [ "$default_imaps" != "$live" ] || fail "$1: IMAPS without SNI served the site certificate (the SNI block was not what selected it)"
}
units() {
  for u in "$timer" nginx postfix dovecot lab-pebble; do
    log "unit $u: enabled=$(systemctl is-enabled "$u" 2>&1 || true) active=$(systemctl is-active "$u" 2>&1 || true)"
  done
}
no_celikpanel_program() {
  [ ! -e /root/agent.test ] || fail "the Agent test binary is still on the system"
  if ps -eo args | grep -iE 'celikpanel|agent\.test' | grep -v grep | grep -q .; then fail "a CelikPanel process runs"; fi
  log "no CelikPanel program on the system (no binary, no process)"
}

case "$stage" in
first)
  log "$PRETTY_NAME ($family), kernel $(uname -r), systemd $(systemctl --version | head -1)"
  if [ "$family" = apt ]; then
    dpkg-query -W certbot python3-acme nginx postfix dovecot-core dovecot-imapd openssl >"$out/package-versions.txt"
  else
    pacman -Q certbot python-acme nginx postfix dovecot openssl >"$out/package-versions.txt"
  fi
  cat "$out/package-versions.txt" | tee -a "$out/run.log"
  log "dovecot $(dovecot --version), postfix $(postconf -h mail_version)"
  cp /etc/hosts "$out/hosts.before"
  printf '127.0.0.1 site.test www.site.test mail.site.test mx.site.test\n' >>/etc/hosts
  # The packaged default site would answer every name on port 80.
  rm -f /etc/nginx/sites-enabled/default
  systemctl cat "$timer" "$service" >"$out/scheduler-units.txt" 2>&1 || true

  log "Pebble test CA on 127.0.0.1:14000 (local only; this guest has no route to the internet)"
  mkdir -p /root/pebble-tls
  cd /root/pebble-tls
  # Python 3.13's strict verification (Arch's certbot) wants a CA with key
  # usage and identifiers on both certificates.
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3 -subj "/CN=cert-mail lab throwaway API CA" \
    -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" \
    -addext "subjectKeyIdentifier=hash" -keyout ca.key -out ca.pem 2>/dev/null
  openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=localhost" -keyout key.pem -out req.pem 2>/dev/null
  printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid\n' >ext.cnf
  openssl x509 -req -in req.pem -CA ca.pem -CAkey ca.key -CAcreateserial -days 3 -extfile ext.cnf -out cert.pem 2>/dev/null
  cd /
  if [ "$family" = apt ]; then
    cp /root/pebble-tls/ca.pem /usr/local/share/ca-certificates/cert-mail-lab-pebble-api.crt
    update-ca-certificates >/dev/null
  else
    cp /root/pebble-tls/ca.pem /etc/ca-certificates/trust-source/anchors/cert-mail-lab-pebble-api.crt
    update-ca-trust
  fi
  # Ten-minute certificates: certbot 5 (Arch) renews when a third of the
  # lifetime is left, certbot 2.9 (Ubuntu) whenever 30 days are left, so both
  # are due within the timer wait of the renew stage.
  cat >/root/pebble.json <<'EOF'
{"pebble": {
  "listenAddress": "127.0.0.1:14000",
  "managementListenAddress": "127.0.0.1:15000",
  "certificate": "/root/pebble-tls/cert.pem",
  "privateKey": "/root/pebble-tls/key.pem",
  "httpPort": 80, "tlsPort": 5001, "ocspResponderURL": "",
  "externalAccountBindingRequired": false,
  "certificateValidityPeriod": 600
}}
EOF
  cat >/etc/systemd/system/lab-pebble.service <<'EOF'
[Unit]
Description=Disposable lab: Pebble test ACME server (never a real CA)
After=network.target
[Service]
Environment=PEBBLE_VA_NOSLEEP=1 PEBBLE_AUTHZREUSE=0
ExecStart=/usr/local/bin/pebble -config /root/pebble.json
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now lab-pebble.service
  for _ in $(seq 1 50); do curl -fsS https://localhost:14000/dir >/dev/null 2>&1 && break; sleep 0.2; done
  curl -fsS https://localhost:14000/dir >>"$out/run.log" || fail "pebble did not start"

  systemctl enable --now nginx
  phase TestRealSystemSiteCertPrepare
  nginx -t >>"$out/run.log" 2>&1 || fail "nginx -t after Prepare"
  systemctl reload nginx
  log "$timer before issuance: enabled=$(systemctl is-enabled "$timer" 2>&1 || true)"
  phase TestRealSystemSiteCertIssue
  log "$timer after issuance: enabled=$(systemctl is-enabled "$timer" 2>&1 || true) active=$(systemctl is-active "$timer" 2>&1 || true)"
  systemctl reload nginx
  phase TestRealSystemSiteCertMail
  systemctl enable postfix dovecot >/dev/null 2>&1
  systemctl restart postfix dovecot
  sleep 2
  phase TestRealSystemSiteCertObserve SITECERT_LABEL=issued SITECERT_EXPECT_STATE=scheduled
  served_all "issued"
  units
  live_fp >/root/sitecert.fp-before
  cp "/etc/letsencrypt/renewal/$(cat /root/sitecert.lineage).conf" "$out/renewal.conf"
  cp /etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates "$out/deploy-hook"
  cp /etc/dovecot/conf.d/98-celikpanel-tls.conf "$out/dovecot-98-celikpanel-tls.conf"
  [ -f /etc/dovecot/dovecot.conf ] && cp /etc/dovecot/dovecot.conf "$out/dovecot.conf"
  cp /etc/postfix/celikpanel_sni "$out/postfix-celikpanel_sni"
  postconf -n >"$out/postconf-n.txt"
  doveconf -n >"$out/doveconf-n.txt" 2>&1
  sha256sum /etc/dovecot/conf.d/98-celikpanel-tls.conf /etc/postfix/celikpanel_sni >/root/mail-config.sha256
  rm -f /root/agent.test
  no_celikpanel_program
  log "stage first done; lab.sh reboots the guest"
  ;;
renew)
  # Same boot as the issuance: Pebble keeps its ACME accounts only in memory,
  # so a reboot of the lab CA (not of a real one) would forget the account.
  no_celikpanel_program
  before=$(cat /root/sitecert.fp-before)
  log "$timer: enabled=$(systemctl is-enabled "$timer") active=$(systemctl is-active "$timer")"
  # Only the schedule changes, and only until the next boot (/run): the unit,
  # its command and certbot's configuration are the package's own.
  mkdir -p "/run/systemd/system/$timer.d"
  printf '[Timer]
OnCalendar=
OnCalendar=*:*:00
RandomizedDelaySec=0
AccuracySec=1s
' >"/run/systemd/system/$timer.d/lab-schedule.conf"
  systemctl daemon-reload
  log "runtime drop-in moves $timer to once a minute; waiting for systemd to start $service by itself"
  systemctl list-timers "$timer" --no-pager >>"$out/run.log" 2>&1 || true
  i=0
  while [ "$(live_fp)" = "$before" ]; do
    i=$((i + 1))
    [ $i -le 120 ] || fail "certbot did not renew within 10 minutes"
    sleep 5
  done
  # Let the deploy hook finish its reloads, then put the schedule back.
  for _ in $(seq 1 60); do
    state=$(systemctl show -p ActiveState --value "$service")
    [ "$state" != activating ] && [ "$state" != active ] && break
    sleep 2
  done
  rm -f "/run/systemd/system/$timer.d/lab-schedule.conf"
  systemctl daemon-reload
  journalctl -u "$service" -u "$timer" --no-pager -b >"$out/journal-certbot.txt" 2>&1 || true
  log "systemd: $(journalctl -u "$service" -b --no-pager -o cat 2>/dev/null | grep -E '^(Starting|Started|Finished)' | tail -2 | tr '
' '|')"
  log "$service: $(systemctl show -p Result -p ExecMainStatus "$service" | tr '
' ' ')"
  tee -a "$out/run.log" <"/var/lib/celikpanel-site-certificates/$(cat /root/sitecert.lineage)"
  cp "/var/lib/celikpanel-site-certificates/$(cat /root/sitecert.lineage)" "$out/deploy-record"
  sleep 2
  served_all "renewed by the operating system's timer"
  [ "$(live_fp)" != "$before" ] || fail "the live certificate did not change"
  live_fp >/root/sitecert.fp-renewed
  sha256sum -c /root/mail-config.sha256 >>"$out/run.log" 2>&1 || fail "the mail configuration changed during the renewal"
  log "mail configuration byte-identical after the renewal; lab.sh reboots the guest"
  ;;
second)
  log "after reboot: boot id $(cat /proc/sys/kernel/random/boot_id), up $(cut -d' ' -f1 /proc/uptime)s"
  no_celikpanel_program
  units
  [ "$(systemctl is-enabled "$timer")" = enabled ] || fail "$timer is not enabled after the reboot"
  [ "$(systemctl is-active "$timer")" = active ] || fail "$timer is not active after the reboot"
  [ ! -e "/run/systemd/system/$timer.d/lab-schedule.conf" ] || fail "the lab schedule drop-in survived"
  systemctl list-timers "$timer" --no-pager >>"$out/run.log" 2>&1 || true
  [ "$(live_fp)" = "$(cat /root/sitecert.fp-renewed)" ] || fail "the live certificate changed across the reboot"
  served_all "after reboot"
  ;;
observe)
  # Copied back once, after the renewal, to read what the Agent concludes.
  phase TestRealSystemSiteCertObserve SITECERT_LABEL=renewed SITECERT_EXPECT_STATE=scheduled \
    SITECERT_EXPECT_NEWER_THAN="$(cat /root/sitecert.notafter)" SITECERT_EXPECT_DEPLOY=reloaded SITECERT_EXPECT_MAIL=reloaded/reloaded
  cp /root/sitecert-state-*.json "$out/"
  rm -f /root/agent.test
  log "stage observe done"
  ;;
*) fail "unknown stage $stage" ;;
esac
