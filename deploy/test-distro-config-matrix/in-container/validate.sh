#!/usr/bin/env bash
# Runs INSIDE the provisioned release container, after the render tests have
# written their cases (internal/configmatrix format) to CASES. For each case:
# back up what it overwrites, copy its files to their real paths, replay the
# commands the product runs (setup.sh), run the program's OWN validator, then
# restore the stock state so the next case starts clean.
#
# usage: validate.sh RELEASE FAMILY CASES RESULTS_TSV
# RESULTS_TSV columns: release case program variant status source message
# status: PASS | WARN (validator succeeded but printed warnings) | FAIL |
#         RENDER-FAILED (the generator refused) | N/A (not written on this release)
set -uo pipefail
LC_ALL=C
export LC_ALL
release=$1 family=$2 cases=$3 results=$4
work=$(mktemp -d)
: >"$results"

oneline() { tr '\t\r' '  ' | sed -e 's/[[:space:]]\+$//' | awk 'NF' | head -c 3000 | sed ':a;N;$!ba;s/\n/ ⏎ /g'; }
record() { # status message
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$release" "$CASE_ID" "$PROGRAM" "$VARIANT" "$1" "$SOURCE" "$(printf '%s' "$2" | oneline)" >>"$results"
    printf '%-14s %-13s %s\n' "$1" "$release" "$CASE_ID" >&2
}
ensure_user() {
    local u
    for u in "$@"; do
        id -u "$u" >/dev/null 2>&1 || useradd -r -M -d /nonexistent -s /usr/sbin/nologin "$u" >/dev/null 2>&1 \
            || useradd -r -M -s /bin/false "$u" >/dev/null 2>&1 || true
    done
}
warnings_in() { grep -Ei 'warn|error|fatal|unknown|deprecated|obsolete|invalid|failed|not supported' || true; }

# --- TLS handshakes on the running daemon (cases with TLS_SNI_EXPECT) -------
# The SHA-256 fingerprint of the certificate a daemon serves for one SNI name
# ("-" sends none). STARTTLS is the protocol name for openssl -starttls.
tls_fingerprint() { # port starttls name
    local args=(-connect "127.0.0.1:$1")
    [[ -n "$2" ]] && args+=(-starttls "$2")
    if [[ "$3" == - ]]; then args+=(-noservername); else args+=(-servername "$3"); fi
    timeout 10 openssl s_client "${args[@]}" </dev/null 2>/dev/null \
        | openssl x509 -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//; s/://g' | tr 'A-F' 'a-f'
}
# Every "name=fingerprint" entry must be what the daemon serves.
tls_expect() { # label port starttls entries
    local entry name want got rc=0 n=0
    for entry in $4; do
        name=${entry%%=*} want=${entry#*=}
        got=$(tls_fingerprint "$2" "$3" "$name")
        n=$((n + 1))
        if [[ "$got" != "$want" ]]; then
            printf '%s: SNI %s on port %s served %s, expected %s\n' "$1" "$name" "$2" "${got:-no certificate}" "${want:0:16}"
            rc=1
        fi
    done
    [[ $rc -eq 0 ]] && printf '%s: %d SNI name(s) on port %s served their own certificate\n' "$1" "$n" "$2"
    return $rc
}
# What `certbot renew` does to a lineage (live links to the next archive
# version), then CelikPanel's site deploy hook exactly as certbot runs it.
# The container has no systemd: a stand-in systemctl answers the hook's two
# questions (is-active, reload) with each program's own command and logs them.
tls_renew_with_hook() { # lineage
    local lineage=$1 f
    for f in cert chain fullchain privkey; do
        ln -sfn "../../archive/$lineage/${f}2.pem" "/etc/letsencrypt/live/$lineage/$f.pem"
    done
    mkdir -p /run/matrix-shim
    cat >/run/matrix-shim/systemctl <<'SHIM'
#!/bin/sh
echo "systemctl $*" >>/var/log/matrix-systemctl.calls
verb=$1; shift
[ "$1" = --quiet ] && shift
case "$verb:${1%.service}" in
is-active:dovecot) pgrep -x dovecot >/dev/null ;;
is-active:postfix) postfix status >/dev/null 2>&1 ;;
is-active:nginx) pgrep -x nginx >/dev/null ;;
reload:dovecot) doveadm reload ;;
reload:postfix) postfix reload >/dev/null 2>&1 ;;
reload:nginx) nginx -s reload ;;
*) exit 1 ;;
esac
SHIM
    chmod 0755 /run/matrix-shim/systemctl
    PATH=/run/matrix-shim:$PATH RENEWED_LINEAGE=/etc/letsencrypt/live/$lineage \
        sh /etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates
}
# The SNI checks, then the renewal and the checks for the renewed lineage.
tls_checks() { # label port starttls record-field
    local rc=0 hook_rc record
    [[ -n "${TLS_SNI_EXPECT:-}" ]] || return 0
    tls_expect "$1" "$2" "$3" "$TLS_SNI_EXPECT" || rc=1
    [[ -n "${TLS_RENEW_LINEAGE:-}" ]] || return $rc
    tls_renew_with_hook "$TLS_RENEW_LINEAGE"; hook_rc=$?
    record=$(tr '\n' ' ' <"/var/lib/celikpanel-site-certificates/$TLS_RENEW_LINEAGE" 2>/dev/null)
    printf 'site deploy hook: exit %s, record: %s\n' "$hook_rc" "${record:-none}"
    grep -q "$4=reloaded" <<<"$record" || { printf 'site deploy hook did not reload %s\n' "$4"; rc=1; }
    sleep 2
    tls_expect "$1 after renewal" "$2" "$3" "$TLS_RENEW_EXPECT" || rc=1
    rm -rf /run/matrix-shim /var/lib/celikpanel-site-certificates
    return $rc
}

# Each validator prints its findings and returns 0 (PASS), 1 (FAIL) or 2 (WARN).
# Warnings the release's stock nginx configuration prints by itself (Arch:
# types_hash) are not the panel's: they are recorded once as a baseline case
# and not repeated on every nginx case.
nginx_baseline=
nginx_warnings() { grep '\[warn\]' | sed -E 's/^.*\[warn\] [0-9#]+: //' | sort -u; }
v_nginx() {
    local out rc warn
    out=$(nginx -t 2>&1); rc=$?
    if [[ $rc -ne 0 ]]; then printf '%s\n' "$out"; return 1; fi
    warn=$(nginx_warnings <<<"$out" | grep -vxF -e "${nginx_baseline:-@none@}" || true)
    [[ -n "$warn" ]] && { printf '%s\n' "$warn"; return 2; }
    return 0
}
# The default HTTPS server, answered for real: nginx -t, then nginx is
# started, TLS is opened for the site's own name (KNOWN_NAME must get the
# site's certificate: nginx is up and serving) and for a name no site has
# (EXPECT_DEFAULT: "reject" = no certificate at all; "placeholder:<CN>" = the
# placeholder certificate, never a site's). nginx is stopped afterwards.
tls_subject() { # server name
    echo | timeout 10 openssl s_client -connect 127.0.0.1:443 -servername "$1" 2>/dev/null \
        | openssl x509 -noout -subject 2>/dev/null
}
v_nginx_tls_default() {
    local rc out known unknown i
    v_nginx; rc=$?
    [[ $rc -eq 1 ]] && return 1
    command -v openssl >/dev/null 2>&1 || { echo "openssl is not installed in the container (provision.sh)"; return 1; }
    # Started in the background with its output in a file: a daemon must not
    # inherit a command-substitution pipe (the harness would wait forever),
    # and a stock nginx.conf may run it in the foreground (daemon off).
    nginx >/var/log/matrix-nginx-start.log 2>&1 </dev/null &
    local started=$!
    for i in 1 2 3 4 5 6 7 8 9 10; do
        timeout 1 bash -c '</dev/tcp/127.0.0.1/443' 2>/dev/null && break
        sleep 0.5
    done
    known=$(tls_subject "$KNOWN_NAME")
    unknown=$(tls_subject unknown-name.example.invalid)
    nginx -s stop >/dev/null 2>&1 || kill "$started" 2>/dev/null
    sleep 1
    pkill -x nginx 2>/dev/null || true
    wait "$started" 2>/dev/null
    out=$(head -c 500 /var/log/matrix-nginx-start.log)
    [[ -n "$out" ]] && printf 'nginx start: %s\n' "$out"
    printf 'TLS for %s: %s; for a name with no site: %s\n' "$KNOWN_NAME" "${known:-no certificate}" "${unknown:-no certificate}"
    if ! grep -Eq "CN ?= ?${KNOWN_NAME//./\\.}\$" <<<"$known"; then
        echo "the site's own name did not get the site's certificate"
        return 1
    fi
    case "${EXPECT_DEFAULT:-}" in
    reject)
        [[ -z "$unknown" ]] || { echo "a name with no site was shown a certificate"; return 1; } ;;
    placeholder:*)
        grep -Eq "CN ?= ?${EXPECT_DEFAULT#placeholder:}\$" <<<"$unknown" \
            || { echo "a name with no site did not get the placeholder certificate"; return 1; } ;;
    *) echo "EXPECT_DEFAULT is not set"; return 1 ;;
    esac
    return $rc
}
fpm_binary() {
    if [[ "$family" == pacman ]]; then echo php-fpm; else echo "php-fpm${PHP_VERSION}"; fi
}
v_php_fpm() {
    local out rc bin
    ensure_user ${USERS:-}
    bin=$(fpm_binary)
    out=$("$bin" -t 2>&1); rc=$?
    printf '%s\n' "$out" | grep -v 'test is successful' || true
    [[ $rc -ne 0 ]] && return 1
    grep -Eq 'WARNING|ERROR' <<<"$out" && return 2
    return 0
}
v_php_ini() {
    local out rc
    v_php_fpm; rc=$?
    [[ $rc -eq 1 ]] && return 1
    out=$(php -c "$TARGET" -d display_startup_errors=1 -r 'echo ini_get("memory_limit"), "\n";' 2>&1)
    if grep -Eqi 'warning|parse error|deprecated' <<<"$out"; then printf '%s\n' "$out"; return 2; fi
    return $rc
}
v_php_lint() {
    local out rc
    out=$(php -l "$TARGET" 2>&1); rc=$?
    [[ $rc -ne 0 ]] && { printf '%s\n' "$out"; return 1; }
    grep -qi 'deprecated\|warning' <<<"$out" && { printf '%s\n' "$out"; return 2; }
    return 0
}
v_postfix() {
    local out rc=0 warn=0 port
    out=$(postfix check 2>&1) || rc=1
    [[ -n "$out" ]] && { printf 'postfix check: %s\n' "$out"; warn=1; }
    out=$(postconf -n 2>&1 >/dev/null)
    [[ -n "$out" ]] && { printf 'postconf -n: %s\n' "$out"; warn=1; }
    out=$(postconf -M 2>&1 >/dev/null)
    [[ -n "$out" ]] && { printf 'postconf -M: %s\n' "$out"; warn=1; }
    out=$(postconf -P 2>&1 >/dev/null)
    [[ -n "$out" ]] && { printf 'postconf -P: %s\n' "$out"; warn=1; }
    # Start the real daemon (logging to a file) and open each listener once:
    # smtpd reads its -o overrides and the TLS/SNI tables only when it runs.
    # What the distro's postfix.service does before starting: on Debian and
    # Ubuntu (configure-instance.sh) the chroot gets hosts/resolv.conf/
    # services; Arch's unit copies nothing, so neither does this harness there
    # (copying them everywhere hid D3, final alpha.113 acceptance run 2).
    if [[ "$family" == apt && -d /var/spool/postfix ]]; then
        mkdir -p /var/spool/postfix/etc
        for f in /etc/hosts /etc/resolv.conf /etc/nsswitch.conf /etc/services /etc/host.conf /etc/localtime; do
            [[ -e "$f" ]] && cp -L "$f" "/var/spool/postfix$f" 2>/dev/null
        done
    fi
    newaliases >/dev/null 2>&1 || true   # Arch's postfix.service runs it first
    postconf -e maillog_file=/var/log/matrix-postfix.log >/dev/null 2>&1
    # The non-SMTP message below is held in the queue, never delivered.
    local nonsmtp
    nonsmtp=$(postconf -h non_smtpd_milters 2>/dev/null)
    [[ -n "$nonsmtp" ]] && postconf -e 'defer_transports=local,virtual,smtp,relay,lmtp,error' >/dev/null 2>&1
    : >/var/log/matrix-postfix.log
    matrix_milters_start || rc=1
    if postfix start >/dev/null 2>&1 </dev/null; then
        sleep 2
        # Port 25 runs a whole envelope so every milter event is sent; 587
        # and 465 are opened as clients connect to them (D3 was logged by
        # postfix/submission/smtpd at connect).
        timeout 10 bash -c "exec 3<>/dev/tcp/127.0.0.1/25; head -c 200 <&3 >/dev/null; printf 'EHLO matrix.invalid\r\n' >&3; sleep 1; printf 'MAIL FROM:<sender@example.invalid>\r\n' >&3; sleep 1; printf 'RCPT TO:<postmaster>\r\n' >&3; sleep 1; printf 'QUIT\r\n' >&3; sleep 1" 2>/dev/null || true
        for port in 587 465; do
            timeout 5 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port; head -c 200 <&3 >/dev/null; printf 'EHLO matrix.invalid\r\n' >&3; sleep 1; printf 'QUIT\r\n' >&3" 2>/dev/null || true
        done
        # Mail made without SMTP (a site's PHP mail(), cron): sendmail ->
        # pickup -> cleanup, which passes non_smtpd_milters from cleanup's
        # own chroot setting. Since 31 Oct 2026 that list holds Rspamd too.
        if [[ -n "$nonsmtp" ]]; then
            printf 'From: matrix@example.invalid\nTo: postmaster@example.invalid\nSubject: matrix non-SMTP\n\nmatrix\n' |
                timeout 10 sendmail -f matrix@example.invalid postmaster@example.invalid 2>&1 | head -3
            for _ in $(seq 1 10); do
                grep -q 'from=<matrix@example.invalid>, size=' /var/log/matrix-postfix.log && break
                sleep 1
            done
            if ! grep -q 'from=<matrix@example.invalid>, size=' /var/log/matrix-postfix.log; then
                printf 'non-SMTP message was not queued through cleanup (non_smtpd_milters = %s)\n' "$nonsmtp"
                rc=1
            fi
            postsuper -d ALL >/dev/null 2>&1
        fi
        sleep 2
        tls_checks "smtpd STARTTLS" 25 smtp mail || rc=1
        postfix stop >/dev/null 2>&1
        sleep 1
    else
        printf 'postfix start failed: %s\n' "$(postfix start 2>&1 | tail -3)"
        rc=1
    fi
    matrix_milters_stop
    # A milter on a unix socket (spamass-milter) has no stand-in here; every
    # inet milter has one, so a failure to reach it is the case's failure.
    out=$(grep -Ei 'milter' /var/log/matrix-postfix.log | grep -Ei 'warning|error|fatal' | grep -v 'Milter service unix:' | sed -E 's/^[A-Za-z]{3} +[0-9]+ [0-9:]+ [^ ]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'milter unreachable: %s\n' "$out"; rc=1; }
    # The non-SMTP test message is held by defer_transports: Postfix logs
    # that through its "error" delivery agent (postfix/error ... deferred
    # transport), which is the intended outcome, not a runtime error.
    noise='Milter service unix:|SSL_accept error|lost connection after CONNECT|milter|status=deferred \(deferred transport\)'
    out=$(grep -Ei 'fatal|error|panic' /var/log/matrix-postfix.log | grep -Ev "$noise" | sed -E 's/^[A-Za-z]{3} +[0-9]+ [0-9:]+ [^ ]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'postfix runtime: %s\n' "$out"; rc=1; }
    out=$(grep -Ei 'warning' /var/log/matrix-postfix.log | grep -v 'maillog_file' | grep -Ev "$noise" | sed -E 's/^[A-Za-z]{3} +[0-9]+ [0-9:]+ [^ ]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'postfix runtime: %s\n' "$out"; warn=1; }
    if [[ -n "$MATRIX_MILTERS" ]]; then
        printf 'milters %s reached from smtp chroot=%s, submission chroot=%s, smtps chroot=%s\n' "$MATRIX_MILTERS" \
            "$(postconf -F smtp/inet/chroot 2>/dev/null | sed 's/.*= //')" \
            "$(postconf -F submission/inet/chroot 2>/dev/null | sed 's/.*= //')" \
            "$(postconf -F smtps/inet/chroot 2>/dev/null | sed 's/.*= //')"
    fi
    if [[ -n "$nonsmtp" ]]; then
        # Postfix's own bounces and notices must not pass these milters: a
        # filter rejecting a bounce would lose it. That holds while
        # internal_mail_filter_classes names no class (the release default;
        # CelikPanel never sets it).
        local classes
        classes=$(postconf -h internal_mail_filter_classes 2>/dev/null)
        printf 'non_smtpd_milters %s reached from cleanup chroot=%s; internal_mail_filter_classes=%s\n' "$nonsmtp" \
            "$(postconf -F cleanup/unix/chroot 2>/dev/null | sed 's/.*= //')" "${classes:-(empty)}"
        if [[ "$classes" =~ bounce|notify ]]; then
            printf 'Postfix bounces would pass non_smtpd_milters (internal_mail_filter_classes=%s)\n' "$classes"
            rc=1
        fi
    fi
    [[ $rc -ne 0 ]] && return 1
    [[ $warn -ne 0 ]] && return 2
    return 0
}
# A real milter on every inet address the case's milter lists name: OpenDKIM
# in verify mode stands in for OpenDKIM and Rspamd alike, so a Postfix
# service that cannot reach an address from its real chroot setting logs it
# (D3) and the case fails. The listener binds exactly the address named; the
# name "localhost" (releases up to alpha.113) means 127.0.0.1 for an inet
# socket, as in OpenDKIM's own "inet:8891@localhost".
MATRIX_MILTERS=
matrix_milters_start() {
    local entry hostport host port conf out
    MATRIX_MILTERS=
    for entry in $(postconf -h smtpd_milters non_smtpd_milters 2>/dev/null | tr ', \t' '\n\n\n' | sort -u); do
        [[ "$entry" == inet:* ]] || continue
        hostport=${entry#inet:} host=${hostport%:*} port=${hostport##*:}
        [[ "$host" == localhost ]] && host=127.0.0.1
        if ! command -v opendkim >/dev/null 2>&1; then
            printf 'no OpenDKIM on this release to stand in for milter %s\n' "$entry"
            return 1
        fi
        conf=/run/matrix-milter-$port.conf
        printf 'Mode\t\tv\nSocket\t\tinet:%s@%s\nSyslog\t\tno\n' "$port" "$host" >"$conf"
        id -u opendkim >/dev/null 2>&1 && printf 'UserID\t\topendkim\n' >>"$conf"
        opendkim -f -x "$conf" >"/run/matrix-milter-$port.log" 2>&1 &
        printf '%s\n' "$!" >"/run/matrix-milter-$port.pid"
        MATRIX_MILTERS="$MATRIX_MILTERS${MATRIX_MILTERS:+, }$entry"
    done
    [[ -z "$MATRIX_MILTERS" ]] && return 0
    sleep 2
    for entry in ${MATRIX_MILTERS//,/ }; do
        port=${entry##*:}
        if ! kill -0 "$(cat "/run/matrix-milter-$port.pid")" 2>/dev/null; then
            printf 'stand-in milter for %s did not start: %s\n' "$entry" "$(tr '\n' ' ' <"/run/matrix-milter-$port.log")"
            return 1
        fi
    done
    return 0
}
matrix_milters_stop() {
    local file pid port
    for file in /run/matrix-milter-*.pid; do
        [[ -f "$file" ]] || continue
        pid=$(cat "$file") port=${file#/run/matrix-milter-} port=${port%.pid}
        kill "$pid" 2>/dev/null
        wait "$pid" 2>/dev/null
        # The next case binds the same port: wait until nothing answers on
        # it, and remove any OpenDKIM process of this stand-in still there.
        for _ in $(seq 1 20); do
            timeout 1 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null || break
            sleep 0.5
        done
        pkill -9 -f "matrix-milter-$port.conf" 2>/dev/null
    done
    rm -f /run/matrix-milter-*.pid /run/matrix-milter-*.conf /run/matrix-milter-*.log
}
# --- Delivered mail read back (cases with MAIL_ROOT and MAIL_USERS) ---------
# A configuration Dovecot accepts can still hide every delivered message:
# Debian 13's Dovecot 2.4 passed doveconf, doveadm user and auth test while
# its stock mail_inbox_path sent INBOX to /var/mail (final alpha.113
# acceptance, 30 Oct 2026). So each case delivers one message the way
# Postfix's virtual(8) does and reads it back through Dovecot's storage
# (doveadm fetch) and through IMAP (LOGIN, SELECT INBOX, FETCH), as mail
# clients and webmail do.
#
# The mailbox owner exactly as the Agent creates it (ensureVmailUser: uid and
# gid 5000) and the mail root as secureEnsureMailRoot leaves it (root:vmail
# 0710).
ensure_vmail() { # uid gid
    getent group vmail >/dev/null 2>&1 || groupadd -g "$2" vmail >/dev/null 2>&1
    [[ "$(getent group vmail | cut -d: -f3)" == "$2" ]] || groupmod -g "$2" vmail >/dev/null 2>&1
    if id -u vmail >/dev/null 2>&1; then
        [[ "$(id -u vmail)" == "$1" ]] || usermod -u "$1" -g "$2" vmail >/dev/null 2>&1
    else
        useradd -r -g "$2" -u "$1" -M -d "${MAIL_ROOT:-/nonexistent}" -s /usr/sbin/nologin vmail >/dev/null 2>&1 \
            || useradd -r -g "$2" -u "$1" -M -s /bin/false vmail >/dev/null 2>&1
    fi
    [[ "$(id -u vmail 2>/dev/null)" == "$1" ]]
}
# One IMAP command on descriptor 7; prints the server's lines up to the tagged
# reply and succeeds only on "<tag> OK".
imap_command() { # tag command
    local line
    printf '%s %s\r\n' "$1" "$2" >&7
    while IFS= read -r -t 10 line <&7; do
        line=${line%$'\r'}
        printf '%s\n' "$line"
        if [[ "$line" == "$1 "* ]]; then
            [[ "$line" == "$1 OK"* ]]
            return
        fi
    done
    printf '%s: no tagged reply within 10 s\n' "$1"
    return 1
}
mail_read_check() { # address password uid gid
    local address=$1 domain=${1#*@} local_part=${1%@*} dir subject file out greeting rc=0
    dir=$MAIL_ROOT/$domain/$local_part
    subject="matrix delivery $local_part $(date +%s) $RANDOM"
    # virtual(8): <base>/<domain>/<user>/{tmp,new,cur} owned by the mailbox
    # owner, 0700, and the message as one file in new/.
    install -d -o "$3" -g "$4" -m 0700 "$MAIL_ROOT/$domain" "$dir" "$dir/tmp" "$dir/new" "$dir/cur" || return 1
    file="$dir/new/$(date +%s).M${RANDOM}P$$.matrix"
    printf 'Return-Path: <sender@example.invalid>\nDelivered-To: %s\nFrom: sender@example.invalid\nTo: %s\nSubject: %s\nMessage-ID: <%s.%s@matrix.invalid>\n\nDelivered by the distro config matrix.\n' \
        "$address" "$address" "$subject" "$RANDOM" "$local_part" >"$file"
    chown "$3:$4" "$file"; chmod 0600 "$file"
    out=$(doveadm fetch -u "$address" hdr.subject mailbox INBOX all 2>&1)
    if ! grep -qF "$subject" <<<"$out"; then
        printf 'delivered message not in INBOX (doveadm fetch -u %s): %s\n' "$address" "${out:-no message}"
        rc=1
    fi
    if exec 7<>/dev/tcp/127.0.0.1/143; then
        IFS= read -r -t 10 greeting <&7
        out=$(imap_command a "LOGIN \"$address\" \"$2\"" && imap_command b "SELECT INBOX" \
            && imap_command c "FETCH 1 (BODY.PEEK[HEADER.FIELDS (SUBJECT)])"; imap_command d LOGOUT)
        exec 7<&- 7>&-
        if ! grep -qF "Subject: $subject" <<<"$out"; then
            printf 'IMAP read of %s INBOX failed (greeting: %s): %s\n' "$address" "${greeting%$'\r'}" "$(head -20 <<<"$out")"
            rc=1
        fi
    else
        printf 'IMAP: nothing answers on 127.0.0.1:143\n'
        rc=1
    fi
    # Webmail creates Sent and Trash beside INBOX; the delivered message must
    # stay in INBOX and INBOX must stay listed.
    out=$(doveadm mailbox create -u "$address" Sent 2>&1) || { printf 'doveadm mailbox create -u %s Sent: %s\n' "$address" "$out"; rc=1; }
    out=$(doveadm mailbox list -u "$address" 2>&1)
    if ! grep -qx 'INBOX' <<<"$out" || ! grep -qx 'Sent' <<<"$out"; then
        printf 'mailboxes of %s after creating Sent: %s\n' "$address" "$out"
        rc=1
    fi
    out=$(doveadm fetch -u "$address" hdr.subject mailbox INBOX all 2>&1)
    grep -qF "$subject" <<<"$out" || { printf 'INBOX of %s lost the delivered message after creating Sent: %s\n' "$address" "$out"; rc=1; }
    [[ $rc -eq 0 ]] && printf 'delivered message read back from %s INBOX (doveadm fetch and IMAP)\n' "$address"
    return $rc
}

# The out-of-office reply as the Agent installs it: CelikPanel's script in
# the mailbox's sieve/ directory, the active link, then two messages from the
# same sender through Dovecot LMTP. Exactly one reply must come out (the
# vacation interval), and both messages must be in INBOX. The Postfix side
# (transport_maps -> private/dovecot-lmtp) is checked by the lab run.
lmtp_send() { # recipient sender subject
    local line
    exec 8<>/dev/tcp/127.0.0.1/2424 || return 1
    IFS= read -r -t 10 line <&8
    for line in "LHLO matrix.invalid" "MAIL FROM:<$2>" "RCPT TO:<$1>" "DATA"; do
        printf '%s\r\n' "$line" >&8
    done
    printf 'From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s.%s@matrix.invalid>\r\n\r\nHello.\r\n.\r\nQUIT\r\n' \
        "$2" "$1" "$3" "$RANDOM" "$$" >&8
    timeout 10 cat <&8 >/var/log/matrix-lmtp.last 2>/dev/null
    exec 8<&- 8>&-
    grep -q '^250 2.0.0 <'"$1"'>' /var/log/matrix-lmtp.last
}
sieve_vacation_check() { # address uid gid
    local address=$1 domain=${1#*@} local_part=${1%@*} dir replies out rc=0
    dir=$MAIL_ROOT/$domain/$local_part
    install -d -o "$2" -g "$3" -m 0700 "$MAIL_ROOT/$domain" "$dir" "$dir/sieve" || return 1
    install -o "$2" -g "$3" -m 0600 /etc/dovecot/matrix-autoresponder.sieve "$dir/sieve/celikpanel-autoresponder.sieve" || return 1
    ln -sfn sieve/celikpanel-autoresponder.sieve "$dir/.dovecot.sieve" && chown -h "$2:$3" "$dir/.dovecot.sieve"
    lmtp_send "$address" "friend@example.invalid" "first" || { printf 'LMTP delivery 1 refused: %s\n' "$(tr -d '\r' </var/log/matrix-lmtp.last)"; rc=1; }
    lmtp_send "$address" "friend@example.invalid" "second" || { printf 'LMTP delivery 2 refused: %s\n' "$(tr -d '\r' </var/log/matrix-lmtp.last)"; rc=1; }
    sleep 1
    replies=$(ls /var/log/matrix-vacation.* 2>/dev/null | wc -l)
    if [[ "$replies" != 1 ]]; then
        printf 'out-of-office: %s replies for two messages from one sender (want 1)\n' "$replies"; rc=1
    elif ! grep -q '^Subject: .*Away' /var/log/matrix-vacation.*; then
        printf 'out-of-office reply has the wrong subject: %s\n' "$(head -20 /var/log/matrix-vacation.*)"; rc=1
    fi
    out=$(doveadm fetch -u "$address" hdr.subject mailbox INBOX all 2>&1)
    { grep -q 'first' <<<"$out" && grep -q 'second' <<<"$out"; } || { printf 'LMTP-delivered messages not in INBOX: %s\n' "$out"; rc=1; }
    [[ $rc -eq 0 ]] && printf 'out-of-office: one reply for two messages, both delivered through LMTP to %s\n' "$address"
    return $rc
}

v_dovecot() {
    local out rc=0 warn=0 u
    if [[ -n "${MAIL_UID:-}" ]]; then
        ensure_vmail "$MAIL_UID" "${MAIL_GID:-$MAIL_UID}" || { printf 'could not create the vmail user with uid %s\n' "$MAIL_UID"; return 1; }
    else
        ensure_user vmail
    fi
    if [[ -n "${MAIL_ROOT:-}" ]]; then
        mkdir -p "$MAIL_ROOT"
        [[ -n "${MAIL_UID:-}" ]] && { chown root:vmail "$MAIL_ROOT"; chmod 0710 "$MAIL_ROOT"; }
    fi
    # On a server Postfix has run and created its private/ spool directory,
    # where the submission SASL socket lives.
    if command -v postfix >/dev/null 2>&1; then
        postfix post-install create-missing >/dev/null 2>&1 || true
        mkdir -p /var/spool/postfix/private
    fi
    out=$(doveconf -n 2>&1 >/dev/null); [[ $? -ne 0 ]] && rc=1
    [[ -n "$out" ]] && { printf 'doveconf -n: %s\n' "$out"; [[ $rc -eq 0 ]] && warn=1; }
    [[ $rc -ne 0 ]] && return 1
    printf 'log_path = /var/log/matrix-dovecot.log\n' >/etc/dovecot/conf.d/zz-matrix-log.conf
    : >/var/log/matrix-dovecot.log
    # Out-of-office cases: a test-only LMTP port the harness can speak to
    # from bash, and a sendmail that records the replies instead of sending.
    if [[ -n "${SIEVE_USER:-}" ]]; then
        printf '#!/bin/sh\ncat >"/var/log/matrix-vacation.$$"\n' >/usr/local/bin/matrix-sendmail
        chmod 0755 /usr/local/bin/matrix-sendmail
        rm -f /var/log/matrix-vacation.*
        printf 'sendmail_path = /usr/local/bin/matrix-sendmail\nservice lmtp {\n  inet_listener lmtp {\n    port = 2424\n  }\n}\n' \
            >/etc/dovecot/conf.d/zz-matrix-sieve.conf
    fi
    # The daemon must not inherit a command-substitution pipe (it would
    # hold it open and the harness would wait forever).
    if dovecot >/var/log/matrix-dovecot-start.log 2>&1 </dev/null; then
        sleep 2
        for u in ${MAIL_USERS:-}; do
            out=$(doveadm user "$u" 2>&1) || { printf 'doveadm user %s: %s\n' "$u" "$out"; rc=1; }
            grep -qi 'warn\|error' <<<"$out" && { printf 'doveadm user %s: %s\n' "$u" "$out"; warn=1; }
            out=$(doveadm auth test "$u" "${MAIL_PASSWORD:-}" 2>&1) || { printf 'doveadm auth test %s: %s\n' "$u" "$out"; rc=1; }
            if [[ -n "${MAIL_ROOT:-}" && -n "${MAIL_UID:-}" ]]; then
                mail_read_check "$u" "${MAIL_PASSWORD:-}" "$MAIL_UID" "${MAIL_GID:-$MAIL_UID}" || rc=1
            fi
        done
        if [[ -n "${SIEVE_USER:-}" ]]; then
            sieve_vacation_check "$SIEVE_USER" "${MAIL_UID:-5000}" "${MAIL_GID:-${MAIL_UID:-5000}}" || rc=1
        fi
        # The delivered test mail goes; the root stays as the Agent made it.
        [[ -n "${MAIL_ROOT:-}" && -n "${MAIL_UID:-}" ]] && find "$MAIL_ROOT" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
        timeout 5 bash -c 'exec 3<>/dev/tcp/127.0.0.1/143; head -c 100 <&3 >/dev/null' 2>/dev/null || true
        tls_checks "IMAPS" 993 "" dovecot || rc=1
        doveadm stop >/dev/null 2>&1 || pkill dovecot
        sleep 1
    else
        printf 'dovecot start: %s\n' "$(cat /var/log/matrix-dovecot-start.log)"; rc=1
    fi
    out=$(grep -Ei 'fatal|error|panic' /var/log/matrix-dovecot.log | sed -E 's/^[A-Za-z]{3} [0-9]+ [0-9:]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'dovecot runtime: %s\n' "$out"; rc=1; }
    # The reload the site deploy hook asked for (tls_checks) is logged as a
    # warning by Dovecot's master; it is the expected effect, not a finding.
    out=$(grep -Ei 'warning' /var/log/matrix-dovecot.log | grep -v 'Killed with signal 15' \
        | grep -v 'SIGHUP received - reloading configuration' | sed -E 's/^[A-Za-z]{3} [0-9]+ [0-9:]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'dovecot runtime: %s\n' "$out"; warn=1; }
    rm -f /etc/dovecot/conf.d/zz-matrix-log.conf /etc/dovecot/conf.d/zz-matrix-sieve.conf /var/log/matrix-vacation.*
    [[ $rc -ne 0 ]] && return 1
    [[ $warn -ne 0 ]] && return 2
    return 0
}
v_bind() {
    local out rc=0 warn=0 entry zone file user=bind
    [[ "$family" == pacman ]] && user=named
    out=$(named-checkconf -z "$TARGET" 2>&1) || rc=1
    [[ $rc -ne 0 ]] && printf 'named-checkconf -z: %s\n' "$out"
    for entry in ${ZONES:-}; do
        zone=${entry%%=*} file=${entry#*=}
        out=$(named-checkzone -k fail -n fail "$zone" "$file" 2>&1) || { printf 'named-checkzone %s: %s\n' "$zone" "$out"; rc=1; }
    done
    [[ $rc -ne 0 ]] && return 1
    # named itself (the catalog-zones consumer is only validated when the
    # server loads it).
    out=$(timeout 6 named -g -u "$user" -c "$TARGET" -p 5353 2>&1)
    if grep -Eqi 'loading configuration: failure|exiting \(due to fatal error\)|fatal' <<<"$out"; then
        # The last lines before the exit, without the start-up banner.
        sed -E 's/^[0-9]{2}-[A-Za-z]{3}-[0-9]{4} [0-9:.]+ //' <<<"$out" \
            | grep -Ev "^(built with|running as|compiled|linked|found [0-9]+ CPU|using [0-9]+ UDP|-+$|BIND 9 is|corporation|support|Inc\.|Please|available|starting BIND|running on|adjusted limit)" \
            | tail -10
        return 1
    fi
    out=$(grep -Ei "warning|deprecated|obsolete|not recommended" <<<"$out" | grep -vi 'none:' | sed -E 's/^[0-9]{2}-[A-Za-z]{3}-[0-9]{4} [0-9:.]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'named: %s\n' "$out"; warn=1; }
    [[ $warn -ne 0 ]] && return 2
    return 0
}
v_powerdns() {
    local out rc=0 warn=0 user=${PDNS_USER:-pdns}
    # Debian/Ubuntu run as pdns; Arch's extra/powerdns as powerdns.
    chown "$user:$user" /var/lib/powerdns/pdns.sqlite3 2>/dev/null || true
    chown "$user:$user" /var/lib/powerdns 2>/dev/null || true
    mkdir -p /run/pdns-matrix
    out=$(timeout 6 pdns_server --daemon=no --guardian=no --disable-syslog --write-pid=no \
        --local-address=127.0.0.1 --local-port=5300 --socket-dir=/run/pdns-matrix 2>&1)
    if grep -Eqi 'fatal|unknown|exiting|error' <<<"$out"; then
        grep -Ei 'fatal|unknown|exiting|error|warning' <<<"$out" | sed -E 's/^[A-Z][a-z]{2} [0-9]+ [0-9:]+ //' | sort -u | head -15
        rc=1
    else
        out=$(grep -Ei 'warning|deprecated|obsolete' <<<"$out" | sed -E 's/^[A-Z][a-z]{2} [0-9]+ [0-9:]+ //' | sort -u)
        [[ -n "$out" ]] && { printf 'pdns_server: %s\n' "$out"; warn=1; }
    fi
    out=$(pdnsutil list-all-zones 2>&1) || { printf 'pdnsutil list-all-zones: %s\n' "$out"; rc=1; }
    [[ $rc -ne 0 ]] && return 1
    [[ $warn -ne 0 ]] && return 2
    return 0
}
v_nftables() {
    local out rc=0
    if [[ -n "${PRECREATE:-}" ]]; then
        out=$(printf '%s' "$PRECREATE" | nft -f - 2>&1) || { printf 'pre-create existing table: %s\n' "$out"; return 1; }
    fi
    out=$(nft -c -f "$TARGET" 2>&1) || rc=1
    [[ -n "$out" ]] && printf '%s\n' "$out"
    nft delete table inet celikpanel_fw >/dev/null 2>&1 || true
    [[ $rc -ne 0 ]] && return 1
    [[ -n "$out" ]] && return 2
    return 0
}
v_systemd_unit() {
    local out rc unit_name exe stubs=()
    unit_name=$(basename "$TARGET")
    # Stand-in executables for binaries the release installs later (the
    # panel's own); a missing distro binary would still be reported.
    while read -r exe; do
        [[ -e "$exe" ]] && continue
        mkdir -p "$(dirname "$exe")"
        printf '#!/bin/sh\nexit 0\n' >"$exe"; chmod 0755 "$exe"; stubs+=("$exe")
    done < <(sed -nE 's/^Exec[A-Za-z]*=[-@+!:]*([^ ]+).*/\1/p' "$TARGET" | grep '^/' | sort -u)
    out=$(systemd-analyze verify --man=no "$TARGET" 2>&1); rc=$?
    for exe in "${stubs[@]}"; do rm -f -- "$exe"; done
    out=$(grep -F -e "$unit_name" -e "$TARGET" <<<"$out" | grep -v 'Failed to put bus name\|Failed to connect to bus' || true)
    [[ -n "$out" ]] && printf '%s\n' "$out"
    if [[ -n "$out" ]] && grep -Eqi 'error|failed|invalid|not found|missing' <<<"$out"; then return 1; fi
    [[ -n "$out" ]] && return 2
    return 0
}
v_shell() {
    local out interp
    interp=$(head -1 "$TARGET" | sed -nE 's/^#! *([^ ]+).*/\1/p')
    [[ -n "$interp" && ! -x "$interp" ]] && { printf 'interpreter %s does not exist on this release\n' "$interp"; return 1; }
    out=$(sh -n "$TARGET" 2>&1) || { printf 'sh -n: %s\n' "$out"; return 1; }
    if [[ "$interp" == */bash ]]; then out=$(bash -n "$TARGET" 2>&1) || { printf 'bash -n: %s\n' "$out"; return 1; }; fi
    return 0
}
v_sudoers() {
    local out
    out=$(visudo -cf "$TARGET" 2>&1) || { printf '%s\n' "$out"; return 1; }
    grep -qi 'warning' <<<"$out" && { printf '%s\n' "$out"; return 2; }
    return 0
}
v_sshd() {
    local out sshd
    ensure_user ${USERS:-}
    sshd=$(command -v sshd || echo /usr/sbin/sshd)
    ssh-keygen -A >/dev/null 2>&1
    mkdir -p /run/sshd
    out=$("$sshd" -t 2>&1) || { printf 'sshd -t: %s\n' "$out"; return 1; }
    [[ -n "$out" ]] && { printf 'sshd -t: %s\n' "$out"; return 2; }
    out=$("$sshd" -T -C "user=${USERS%% *},host=primary.example,addr=192.0.2.10" 2>&1 >/dev/null) \
        || { printf 'sshd -T (Match evaluated): %s\n' "$out"; return 1; }
    [[ -n "$out" ]] && { printf 'sshd -T: %s\n' "$out"; return 2; }
    return 0
}
v_apt() {
    local out
    out=$(apt-cache policy 2>&1 >/dev/null)
    out+=$(apt-config dump 2>&1 >/dev/null)
    [[ -z "$out" ]] && return 0
    printf '%s\n' "$out"
    grep -q '^E:' <<<"$out" && return 1
    return 2
}
v_mariadb() {
    local out bin
    bin=$(command -v mariadbd || command -v mysqld || echo /usr/sbin/mariadbd)
    out=$("$bin" --user=mysql --help --verbose 2>&1 >/dev/null)
    [[ -z "$out" ]] && return 0
    printf '%s\n' "$out" | grep -Ei 'error|unknown|warning' | head -10
    grep -Eqi 'error|unknown variable|unknown option' <<<"$out" && return 1
    return 2
}

if command -v nginx >/dev/null 2>&1; then
    nginx_baseline=$(nginx -t 2>&1 | nginx_warnings)
    if [[ -n "$nginx_baseline" ]]; then
        CASE_ID=nginx.stock-baseline PROGRAM=nginx SOURCE=- \
            VARIANT="the release's own nginx.conf before any CelikPanel file" \
            record WARN "stock configuration, not CelikPanel's: $nginx_baseline"
    fi
fi

for dir in "$cases"/*/; do
    [[ -f "$dir/case.env" ]] || continue
    CASE_ID=$(basename "$dir")
    unset PROGRAM VARIANT VALIDATOR SOURCE TARGET REASON OWNERS RESTORE_DIRS PDNS_USER USERS PHP_VERSION ZONES PRECREATE MAIL_USERS MAIL_PASSWORD MAIL_ROOT MAIL_UID MAIL_GID NOTE \
        KNOWN_NAME EXPECT_DEFAULT TLS_SNI_EXPECT TLS_RENEW_LINEAGE TLS_RENEW_EXPECT
    # shellcheck disable=SC1090,SC1091
    . "$dir/case.env"
    case "$VALIDATOR" in
    not-applicable) record "N/A" "${REASON:-}"; continue ;;
    render-failed) record "RENDER-FAILED" "${REASON:-}"; continue ;;
    esac
    fn="v_${VALIDATOR//-/_}"
    if ! declare -F "$fn" >/dev/null; then record FAIL "harness has no validator '$VALIDATOR'"; continue; fi

    # Save what the case overwrites, then put the rendered files in place.
    : >"$work/existing"
    while read -r p; do [[ -n "$p" && ( -e "$p" || -L "$p" ) ]] && printf '%s\n' "${p#/}" >>"$work/existing"; done <"$dir/files.list"
    for d in ${RESTORE_DIRS:-}; do [[ -d "$d" ]] && printf '%s\n' "${d#/}" >>"$work/existing"; done
    tar -C / -cpf "$work/backup.tar" --files-from "$work/existing" 2>/dev/null || tar -C / -cpf "$work/backup.tar" --files-from /dev/null
    # Existing directories keep their distro owner and mode (named, postfix
    # and dovecot check them).
    tar -C "$dir/root" -cf - . | tar -C / -xpf - --no-overwrite-dir
    for spec in ${OWNERS:-}; do chown "${spec#*=}" "${spec%%=*}" 2>/dev/null || true; done

    status=PASS
    message=""
    if ! setup_out=$(bash "$dir/setup.sh" 2>&1); then
        status=FAIL
        message="applying the panel's commands failed: $setup_out"
    else
        message=$("$fn" 2>&1); rc=$?
        [[ -n "$setup_out" ]] && message="setup: $setup_out"$'\n'"$message"
        case $rc in 0) status=PASS ;; 2) status=WARN ;; *) status=FAIL ;; esac
    fi
    record "$status" "$message"

    # Back to the stock state.
    while read -r p; do [[ -n "$p" ]] && rm -rf -- "$p"; done <"$dir/files.list"
    for d in ${RESTORE_DIRS:-}; do [[ -d "$d" ]] && rm -rf -- "$d"; done
    tar -C / -xpf "$work/backup.tar" 2>/dev/null || true
done
rm -rf -- "$work"
