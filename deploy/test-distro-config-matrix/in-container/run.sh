#!/usr/bin/env bash
# Runs INSIDE a clean, disposable container of one release:
#   1. install the release's packages (provision.sh),
#   2. record the versions actually installed,
#   3. run the render tests (static test binaries built by the host) so the
#      product's own generators write their cases for THIS release,
#   4. validate every case with the program's own validator (validate.sh).
# usage: run.sh RELEASE FAMILY CODENAME
# mounts: /src (repository, read-only), /matrix-bin (test binaries), /out (results)
set -euo pipefail
LC_ALL=C
export LC_ALL
release=$1 family=$2 codename=$3
here=/src/deploy/test-distro-config-matrix
trap 'echo "run.sh: command failed at line $LINENO: $BASH_COMMAND" >&2' ERR

bash "$here/in-container/provision.sh" "$family"

# Installed versions (what was validated), next to the checked-in table.
installed_version() {
    if [[ "$family" == apt ]]; then dpkg-query -W -f='${Version}' -- "$1" 2>/dev/null || true
    else { pacman -Q -- "$1" 2>/dev/null || true; } | awk '{print $2}'; fi
}
: >/out/installed.tsv
while read -r program apt_pkg pacman_pkg; do
    [[ -z "$program" || "$program" == \#* ]] && continue
    pkg=$apt_pkg; [[ "$family" == pacman ]] && pkg=$pacman_pkg
    [[ "$pkg" == - ]] && continue
    if [[ "$family" == apt && "$pkg" == php-fpm ]]; then
        pkg=$(dpkg-query -W -f='${Package}\n' 'php*-fpm' 2>/dev/null | grep -E '^php[0-9.]+-fpm$' | head -1 || echo php-fpm)
    fi
    v=$(installed_version "$pkg")
    [[ -n "$v" ]] && printf '%s\t%s\t%s\n' "$program" "$pkg" "$v" >>/out/installed.tsv
done <"$here/packages.list"

if [[ "$family" == apt ]]; then
    php_version=$(ls /etc/php 2>/dev/null | grep -E '^[0-9]+\.[0-9]+$' | sort -V | tail -1)
else
    php_version=$(php -r 'echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;')
fi

rm -rf /out/cases
mkdir -p /out/cases
export CELIKPANEL_CONFIG_MATRIX_OUT=/out/cases CELIKPANEL_CONFIG_MATRIX_REPO=/src \
    CELIKPANEL_CONFIG_MATRIX_RELEASE=$release CELIKPANEL_CONFIG_MATRIX_FAMILY=$family \
    CELIKPANEL_CONFIG_MATRIX_PHP_VERSION=$php_version
: >/out/render.log
for bin in /matrix-bin/*.test; do
    name=$(basename "$bin" .test)
    pkgdir=/src/${name//-//}
    # The render test only reads stock files and writes /out/cases.
    if ! (cd "$pkgdir" && "$bin" -test.run '^TestRenderDistroConfigMatrix$' -test.v -test.count=1) >>/out/render.log 2>&1; then
        printf 'render test %s failed (see render.log)\n' "$name" >&2
        printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$release" "render.$name" "render" "render test binary" "RENDER-FAILED" "$name" \
            "$(grep -E -- '--- FAIL|_test.go:[0-9]+:' /out/render.log | tail -5 | tr '\t\n' '  ')" >>/out/render-failures.tsv
    fi
done

bash "$here/in-container/validate.sh" "$release" "$family" /out/cases /out/results.tsv
[[ -f /out/render-failures.tsv ]] && cat /out/render-failures.tsv >>/out/results.tsv
exit 0
