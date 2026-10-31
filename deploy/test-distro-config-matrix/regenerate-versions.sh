#!/usr/bin/env bash
# Regenerate versions/<release>.json: the exact package version of every
# program in packages.list on every release in releases.list, read from that
# release's own package index inside a clean container.
#
# This is the only part of the distro config matrix that needs the network
# (container images and the distros' official package indexes). Tests and the
# validator harness read the checked-in JSON; they never call this.
#
# usage: regenerate-versions.sh [RELEASE...]     (default: every release)
# env:   CONTAINER_RUNTIME=podman|docker (default: first one found)
set -euo pipefail
LC_ALL=C
export LC_ALL
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
runtime=${CONTAINER_RUNTIME:-}
if [[ -z "$runtime" ]]; then
    for candidate in podman docker; do
        command -v "$candidate" >/dev/null 2>&1 && { runtime=$candidate; break; }
    done
fi
[[ -n "$runtime" ]] || { echo "no container runtime (podman or docker) found" >&2; exit 2; }

want=("$@")
selected() {
    [[ ${#want[@]} -eq 0 ]] && return 0
    local r
    for r in "${want[@]}"; do [[ "$r" == "$1" ]] && return 0; done
    return 1
}

mkdir -p "$here/versions"
status=0
while read -r name image family codename; do
    [[ -z "$name" || "$name" == \#* ]] && continue
    selected "$name" || continue
    printf '== %s (%s)\n' "$name" "$image" >&2
    tmp=$(mktemp)
    if "$runtime" run --rm -v "$here:/matrix:ro" "$image" \
        bash /matrix/in-container/query-packages.sh "$name" "$family" "$codename" /matrix/packages.list >"$tmp"; then
        mv -- "$tmp" "$here/versions/$name.json"
    else
        rm -f -- "$tmp"
        echo "failed to read the package index of $name" >&2
        status=1
    fi
done <"$here/releases.list"
exit $status
