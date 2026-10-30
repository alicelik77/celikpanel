#!/usr/bin/env bash
# Runs INSIDE a clean, disposable release container. Installs, from the
# release's own package repositories, every program whose configuration the
# matrix validates, without starting any service (policy-rc.d / no systemd).
# usage: provision.sh FAMILY
set -euo pipefail
LC_ALL=C
export LC_ALL
family=$1

case "$family" in
apt)
    export DEBIAN_FRONTEND=noninteractive
    printf '#!/bin/sh\nexit 101\n' >/usr/sbin/policy-rc.d
    chmod 0755 /usr/sbin/policy-rc.d
    printf 'postfix postfix/main_mailer_type select Internet Site\npostfix postfix/mailname string matrix.example.test\n' | debconf-set-selections
    apt-get update -qq
    # Validators and the programs the panel configures. Recommends are left
    # out so only the named packages and their dependencies are measured.
    packages=(
        nginx php-fpm php-cli
        postfix
        dovecot-core dovecot-imapd dovecot-pop3d dovecot-lmtpd
        bind9 bind9-utils
        pdns-server pdns-backend-sqlite3
        nftables systemd sudo openssh-server mariadb-server
        ca-certificates procps
        # A validator's TLS client only (v_nginx_tls_default); the panel
        # itself never needs the openssl command.
        openssl
    )
    available=()
    for p in "${packages[@]}"; do
        if apt-cache show -- "$p" >/dev/null 2>&1; then available+=("$p"); else echo "provision: $p is not packaged on this release" >&2; fi
    done
    apt-get install -y -qq --no-install-recommends "${available[@]}" >/var/log/matrix-provision.log 2>&1 \
        || { tail -40 /var/log/matrix-provision.log >&2; exit 1; }
    ;;
pacman)
    pacman -Syu --noconfirm --needed nginx php php-fpm postfix dovecot bind nftables systemd sudo openssh mariadb diffutils openssl \
        >/var/log/matrix-provision.log 2>&1 || { tail -40 /var/log/matrix-provision.log >&2; exit 1; }
    ;;
*) echo "unknown family: $family" >&2; exit 2 ;;
esac
command -v systemd-analyze >/dev/null 2>&1 || echo "provision: systemd-analyze is unavailable" >&2
