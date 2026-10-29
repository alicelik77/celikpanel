#!/usr/bin/env bash
# Distro config matrix: render every configuration file the panel and agent
# write, with the product's own generators, on every supported release, and
# check it with each program's OWN validator (nginx -t, php-fpm -t, postfix
# check + a real smtpd, doveconf -n + doveadm, named-checkconf/-checkzone +
# named -g, pdns_server, nft -c, systemd-analyze verify, sh -n, visudo -c,
# sshd -t/-T, apt-cache/apt-config, mariadbd --help --verbose).
#
# Why: a directive valid on one release's package version can be unknown on
# another's (nginx `http2 on;` needs 1.25.1; Ubuntu 24.04 ships 1.24 and every
# HTTPS site failed). Tests that run on one distribution cannot see that.
#
# Per release (deploy/test-distro-config-matrix/releases.list) a clean
# container is started, the release's packages are installed from its official
# repositories, the render test binaries run INSIDE it (so the Dovecot dialect,
# Postfix map type, PHP-FPM layout and the stock nginx.conf, named.conf.options,
# pdns.conf, 10-auth.conf, php.ini, sshd_config are the release's own) and
# every case is validated, then restored. Nothing on the host is changed; no
# CelikPanel server, licence service or CA is contacted. The network is used
# for container images and the distributions' package mirrors only.
#
# usage: deploy/test-distro-config-matrix.sh [RELEASE...]
# env:   CONTAINER_RUNTIME=podman|docker   (default: first found)
#        GO=go                             (toolchain used to build the render tests)
#        MATRIX_OUT=DIR                    (default: ./distro-config-matrix-out)
#        MATRIX_BIN=DIR                    (prebuilt render test binaries; skips the build)
#        MATRIX_JOBS=N                     (releases in parallel, default 2)
# exit:  0 when every failure is listed in known-failures.list and every
#        listed failure still fails; 1 otherwise; 2 on harness errors.
set -euo pipefail
LC_ALL=C
export LC_ALL
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
matrix=$repo/deploy/test-distro-config-matrix
out=${MATRIX_OUT:-$PWD/distro-config-matrix-out}
jobs=${MATRIX_JOBS:-2}
runtime=${CONTAINER_RUNTIME:-}
if [[ -z "$runtime" ]]; then
    for candidate in podman docker; do command -v "$candidate" >/dev/null 2>&1 && { runtime=$candidate; break; }; done
fi
[[ -n "$runtime" ]] || { echo "no container runtime (podman or docker) found" >&2; exit 2; }
mkdir -p "$out"
out=$(cd -- "$out" && pwd)

render_packages=(cmd/agent internal/services internal/dnspeerenrollowner internal/recoveryruntime)
bin=${MATRIX_BIN:-}
if [[ -z "$bin" ]]; then
    bin=$out/bin
    rm -rf -- "$bin"
    mkdir -p "$bin"
    go=${GO:-go}
    for pkg in "${render_packages[@]}"; do
        echo "building render test ./$pkg" >&2
        (cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=-buildvcs=false "$go" test -c \
            -o "$bin/${pkg//\//-}.test" "./$pkg")
    done
fi

want=("$@")
selected() {
    [[ ${#want[@]} -eq 0 ]] && return 0
    local r
    for r in "${want[@]}"; do [[ "$r" == "$1" ]] && return 0; done
    return 1
}

run_release() {
    local name=$1 image=$2 family=$3 codename=$4 dir=$out/$1
    rm -rf -- "$dir"
    mkdir -p "$dir"
    echo "== $name: starting $image" >&2
    if "$runtime" run --rm --cap-add NET_ADMIN \
        -v "$repo:/src:ro" -v "$bin:/matrix-bin:ro" -v "$dir:/out" "$image" \
        bash /src/deploy/test-distro-config-matrix/in-container/run.sh "$name" "$family" "$codename" \
        >"$dir/container.log" 2>&1; then
        echo "== $name: done" >&2
    else
        echo "== $name: the container run failed (see $dir/container.log)" >&2
        printf '%s\tharness\tharness\tcontainer run\tFAIL\t-\t%s\n' "$name" \
            "$(tail -5 "$dir/container.log" | tr '\t\n' '  ')" >>"$dir/results.tsv"
    fi
}

running=0
while read -r name image family codename; do
    [[ -z "$name" || "$name" == \#* ]] && continue
    selected "$name" || continue
    run_release "$name" "$image" "$family" "$codename" &
    running=$((running + 1))
    if [[ $running -ge $jobs ]]; then wait -n || true; running=$((running - 1)); fi
done <"$matrix/releases.list"
wait || true

# ---------------------------------------------------------------- report ---
results=$out/results.tsv
: >"$results"
for f in "$out"/*/results.tsv; do [[ -f "$f" ]] && cat "$f" >>"$results"; done

# Version drift: what was installed vs the checked-in table.
drift=$out/version-drift.txt
: >"$drift"
for f in "$out"/*/installed.tsv; do
    [[ -f "$f" ]] || continue
    rel=$(basename "$(dirname "$f")")
    table=$matrix/versions/$rel.json
    while IFS=$'\t' read -r program pkg version; do
        expected=$(grep -o "\"$program\": {\"package\": \"[^\"]*\", \"version\": \"[^\"]*\"" "$table" 2>/dev/null | sed -E 's/.*"version": "([^"]*)"/\1/' || true)
        [[ -n "$expected" && "$expected" != "$version" ]] && \
            printf '%s: %s installed %s, versions/%s.json says %s (run regenerate-versions.sh)\n' "$rel" "$program" "$version" "$rel" "$expected" >>"$drift"
    done <"$f"
done

known=$matrix/known-failures.list
new_failures=0 stale=0
{
    printf '# Distro config matrix report\n\n'
    printf '| release | PASS | WARN | FAIL | RENDER-FAILED | N/A |\n|---|---|---|---|---|---|\n'
    for rel in $(cut -f1 "$results" | sort -u); do
        printf '| %s |' "$rel"
        for s in PASS WARN FAIL RENDER-FAILED N/A; do
            printf ' %s |' "$(awk -F'\t' -v r="$rel" -v s="$s" '$1==r && $5==s' "$results" | wc -l)"
        done
        printf '\n'
    done
    printf '\n## Failures and warnings\n\n'
    awk -F'\t' '$5=="FAIL" || $5=="RENDER-FAILED" || $5=="WARN" {printf "- **%s** `%s` %s (%s) — %s\n  - source: %s\n  - %s\n", $5, $1, $2, $4, $3, $6, $7}' "$results"
    printf '\n## Not applicable\n\n'
    awk -F'\t' '$5=="N/A" {printf "- `%s` %s: %s\n", $1, $2, $7}' "$results"
    if [[ -s "$drift" ]]; then printf '\n## Version drift\n\n'; sed 's/^/- /' "$drift"; fi
} >"$out/report.md"

is_known() { # release case status
    [[ -f "$known" ]] || return 1
    local r c s
    while read -r r c s _; do
        [[ -z "$r" || "$r" == \#* ]] && continue
        # shellcheck disable=SC2053
        [[ "$1" == $r && "$2" == "$c" && "$3" == "$s" ]] && return 0
    done <"$known"
    return 1
}
while IFS=$'\t' read -r rel id _ _ status _ _; do
    [[ "$status" == FAIL || "$status" == RENDER-FAILED ]] || continue
    if ! is_known "$rel" "$id" "$status"; then
        printf 'NEW %s: %s %s\n' "$status" "$rel" "$id" >&2
        new_failures=$((new_failures + 1))
    fi
done <"$results"
if [[ -f "$known" ]]; then
    while read -r r c s _; do
        [[ -z "$r" || "$r" == \#* ]] && continue
        # Only releases that ran in this invocation can make an entry stale.
        awk -F'\t' -v c="$c" '$2==c' "$results" | while IFS=$'\t' read -r rel id _ _ status _ _; do
            # shellcheck disable=SC2053
            [[ "$rel" == $r && "$status" != "$s" ]] && printf 'STALE known failure (now %s): %s %s\n' "$status" "$rel" "$id" >&2
        done
    done <"$known" 2>"$out/stale.txt"
    stale=$(wc -l <"$out/stale.txt")
    cat "$out/stale.txt" >&2
fi

cat "$out/report.md"
echo >&2
echo "report: $out/report.md   results: $results" >&2
if [[ $new_failures -gt 0 || $stale -gt 0 ]]; then
    echo "distro config matrix: $new_failures new failure(s), $stale stale known-failure entr(y/ies)" >&2
    exit 1
fi
exit 0
