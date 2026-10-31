#!/usr/bin/env bash
# Runs INSIDE a clean release container. Reads the distro package index and
# prints the candidate version of every program in packages.list as JSON.
# usage: query-packages.sh RELEASE FAMILY CODENAME PACKAGES_LIST
# The only network access is the distro's own package index (apt-get update /
# pacman -Sy) from the image's configured official mirrors.
set -euo pipefail
LC_ALL=C
export LC_ALL
release=$1 family=$2 codename=$3 list=$4

upstream() { sed -E 's/^[0-9]+://; s/-[^-]*$//; s/[+~].*$//' <<<"$1"; }
json_escape() { sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' <<<"$1"; }

case "$family" in
apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null
    candidate() {
        apt-cache policy -- "$1" 2>/dev/null | awk '/Candidate:/ {print $2; exit}'
    }
    # A metapackage (php-fpm, postgresql) is resolved to the versioned
    # package it pulls in, because that is the program whose syntax matters.
    resolve() {
        case "$1" in
        php-fpm) apt-cache depends -- php-fpm 2>/dev/null | awk '/Depends: php[0-9.]+-fpm/ {print $2; exit}' ;;
        postgresql) apt-cache depends -- postgresql 2>/dev/null | awk '/Depends: postgresql-[0-9]+$/ {print $2; exit}' ;;
        *) printf '%s\n' "$1" ;;
        esac
    }
    os_version=$(. /etc/os-release && printf '%s' "$VERSION_ID")
    ;;
pacman)
    pacman -Sy --noconfirm >/dev/null 2>&1
    candidate() { pacman -Si -- "$1" 2>/dev/null | awk -F': ' '/^Version/ {print $2; exit}'; }
    resolve() { printf '%s\n' "$1"; }
    os_version=rolling
    ;;
*) echo "unknown family: $family" >&2; exit 2 ;;
esac

printf '{\n  "release": "%s",\n  "family": "%s",\n  "codename": "%s",\n  "os_version": "%s",\n' \
    "$release" "$family" "$codename" "$os_version"
printf '  "index_read": "%s",\n  "packages": {\n' "$(date -u +%Y-%m-%d)"
first=1
while read -r program apt_pkg pacman_pkg; do
    [[ -z "$program" || "$program" == \#* ]] && continue
    if [[ "$family" == apt ]]; then pkg=$apt_pkg; else pkg=$pacman_pkg; fi
    real=- version= up=
    if [[ "$pkg" != - ]]; then
        real=$(resolve "$pkg")
        [[ -n "$real" ]] || real=$pkg
        version=$(candidate "$real")
        [[ "$version" == "(none)" ]] && version=
        [[ -n "$version" ]] && up=$(upstream "$version")
    fi
    [[ $first -eq 1 ]] || printf ',\n'
    first=0
    if [[ -n "$version" ]]; then
        printf '    "%s": {"package": "%s", "version": "%s", "upstream": "%s"}' \
            "$program" "$(json_escape "$real")" "$(json_escape "$version")" "$(json_escape "$up")"
    else
        printf '    "%s": {"package": "%s", "version": null, "upstream": null}' "$program" "$(json_escape "$pkg")"
    fi
done <"$list"
printf '\n  }\n}\n'
