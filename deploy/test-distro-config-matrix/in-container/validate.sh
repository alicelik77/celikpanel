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

# Each validator prints its findings and returns 0 (PASS), 1 (FAIL) or 2 (WARN).
v_nginx() {
    local out rc
    out=$(nginx -t 2>&1); rc=$?
    printf '%s\n' "$out"
    [[ $rc -ne 0 ]] && return 1
    grep -q '\[warn\]' <<<"$out" && return 2
    return 0
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
    postconf -e maillog_file=/var/log/matrix-postfix.log >/dev/null 2>&1
    : >/var/log/matrix-postfix.log
    if postfix start >/dev/null 2>&1 </dev/null; then
        sleep 2
        for port in 25 587 465; do
            timeout 5 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port; head -c 200 <&3 >/dev/null; printf 'QUIT\r\n' >&3" 2>/dev/null || true
        done
        sleep 2
        postfix stop >/dev/null 2>&1
        sleep 1
    else
        printf 'postfix start failed: %s\n' "$(postfix start 2>&1 | tail -3)"
        rc=1
    fi
    noise='connect to Milter service|host or service localhost:|SSL_accept error|lost connection after CONNECT'
    out=$(grep -Ei 'fatal|error|panic' /var/log/matrix-postfix.log | grep -Ev "$noise" | sed -E 's/^[A-Za-z]{3} +[0-9]+ [0-9:]+ [^ ]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'postfix runtime: %s\n' "$out"; rc=1; }
    out=$(grep -Ei 'warning' /var/log/matrix-postfix.log | grep -v 'maillog_file' | grep -Ev "$noise" | sed -E 's/^[A-Za-z]{3} +[0-9]+ [0-9:]+ [^ ]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'postfix runtime: %s\n' "$out"; warn=1; }
    [[ $rc -ne 0 ]] && return 1
    [[ $warn -ne 0 ]] && return 2
    return 0
}
v_dovecot() {
    local out rc=0 warn=0 u
    ensure_user vmail
    [[ -n "${MAIL_ROOT:-}" ]] && mkdir -p "$MAIL_ROOT"
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
    # The daemon must not inherit a command-substitution pipe (it would
    # hold it open and the harness would wait forever).
    if dovecot >/var/log/matrix-dovecot-start.log 2>&1 </dev/null; then
        sleep 2
        for u in ${MAIL_USERS:-}; do
            out=$(doveadm user "$u" 2>&1) || { printf 'doveadm user %s: %s\n' "$u" "$out"; rc=1; }
            grep -qi 'warn\|error' <<<"$out" && { printf 'doveadm user %s: %s\n' "$u" "$out"; warn=1; }
            out=$(doveadm auth test "$u" "${MAIL_PASSWORD:-}" 2>&1) || { printf 'doveadm auth test %s: %s\n' "$u" "$out"; rc=1; }
        done
        timeout 5 bash -c 'exec 3<>/dev/tcp/127.0.0.1/143; head -c 100 <&3 >/dev/null' 2>/dev/null || true
        doveadm stop >/dev/null 2>&1 || pkill dovecot
        sleep 1
    else
        printf 'dovecot start: %s\n' "$(cat /var/log/matrix-dovecot-start.log)"; rc=1
    fi
    out=$(grep -Ei 'fatal|error|panic' /var/log/matrix-dovecot.log | sed -E 's/^[A-Za-z]{3} [0-9]+ [0-9:]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'dovecot runtime: %s\n' "$out"; rc=1; }
    out=$(grep -Ei 'warning' /var/log/matrix-dovecot.log | grep -v 'Killed with signal 15' | sed -E 's/^[A-Za-z]{3} [0-9]+ [0-9:]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'dovecot runtime: %s\n' "$out"; warn=1; }
    rm -f /etc/dovecot/conf.d/zz-matrix-log.conf
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
        grep -Ei 'error|fail|fatal|unknown|not ' <<<"$out" | sed -E 's/^[0-9]{2}-[A-Za-z]{3}-[0-9]{4} [0-9:.]+ //' | head -15
        return 1
    fi
    out=$(grep -Ei "warning|deprecated|obsolete|not recommended" <<<"$out" | grep -vi 'none:' | sed -E 's/^[0-9]{2}-[A-Za-z]{3}-[0-9]{4} [0-9:.]+ //' | sort -u)
    [[ -n "$out" ]] && { printf 'named: %s\n' "$out"; warn=1; }
    [[ $warn -ne 0 ]] && return 2
    return 0
}
v_powerdns() {
    local out rc=0 warn=0
    chown pdns:pdns /var/lib/powerdns/pdns.sqlite3 2>/dev/null || true
    chown pdns:pdns /var/lib/powerdns 2>/dev/null || true
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

for dir in "$cases"/*/; do
    [[ -f "$dir/case.env" ]] || continue
    CASE_ID=$(basename "$dir")
    unset PROGRAM VARIANT VALIDATOR SOURCE TARGET REASON OWNERS RESTORE_DIRS USERS PHP_VERSION ZONES PRECREATE MAIL_USERS MAIL_PASSWORD MAIL_ROOT NOTE
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
