#!/bin/bash
# Supported-system gate: only the systems on https://celikpanel.net/systems.html
# may continue (Debian 12/13, Ubuntu 22.04/24.04, Arch Linux). Anything else is
# refused with a clear English/Turkish message before anything is changed.
# Desteklenen sistem kapısı: yalnız systems.html listesindeki sistemler devam
# eder; diğerleri bir şey değiştirilmeden açık iki dilli iletiyle reddedilir.
#
# Needs only bash (and dash when present). It reads fake os-release files and
# never touches the real system.
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
INSTALL="$ROOT/install.sh"
GET="$ROOT/download-portal/get.sh"

die() {
    echo "unsupported platform gate test failed: $*" >&2
    exit 1
}

# Print one top-level function (header "name() {" up to the closing "}").
extract_function_source() {
    local file=$1 function_name=$2
    awk -v header="$function_name() {" '
        $0 == header { inside = 1; found = 1 }
        inside { print }
        inside && $0 == "}" { closed = 1; exit }
        END { if (!found || !closed) exit 1 }
    ' "$file"
}

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
work=$tmp/work
mkdir "$work"

# Pull the production functions out of both scripts.
install_defs=$tmp/install-defs.sh
get_defs=$tmp/get-defs.sh
{
    grep -m1 '^c() ' "$INSTALL"
    grep -m1 '^die() ' "$INSTALL"
    for name in bilingual parse_bootstrap_os_release_scalar parse_bootstrap_os_release \
        bootstrap_platform_display_name require_supported_platform; do
        extract_function_source "$INSTALL" "$name" \
            || die "install.sh has no complete function: $name"
    done
} > "$install_defs"
{
    for name in message fail os_release_value require_supported_platform; do
        extract_function_source "$GET" "$name" \
            || die "get.sh has no complete function: $name"
    done
} > "$get_defs"

# Real-looking os-release files. Fields are as the distributions ship them.
write_release() {
    local name=$1
    case "$name" in
        debian12) printf '%s\n' 'PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"' 'NAME="Debian GNU/Linux"' 'VERSION_ID="12"' 'ID=debian' ;;
        debian13) printf '%s\n' 'PRETTY_NAME="Debian GNU/Linux 13 (trixie)"' 'NAME="Debian GNU/Linux"' 'VERSION_ID="13"' 'ID=debian' ;;
        ubuntu2204) printf '%s\n' 'PRETTY_NAME="Ubuntu 22.04.5 LTS"' 'NAME="Ubuntu"' 'VERSION_ID="22.04"' 'ID=ubuntu' 'ID_LIKE=debian' ;;
        ubuntu2404) printf '%s\n' 'PRETTY_NAME="Ubuntu 24.04.1 LTS"' 'NAME="Ubuntu"' 'VERSION_ID="24.04"' 'ID=ubuntu' 'ID_LIKE=debian' ;;
        arch) printf '%s\n' 'NAME="Arch Linux"' 'PRETTY_NAME="Arch Linux"' 'ID=arch' 'BUILD_ID=rolling' ;;
        kali) printf '%s\n' 'PRETTY_NAME="Kali GNU/Linux Rolling"' 'NAME="Kali GNU/Linux"' 'VERSION_ID="2025.3"' 'ID=kali' 'ID_LIKE=debian' ;;
        debian11) printf '%s\n' 'PRETTY_NAME="Debian GNU/Linux 11 (bullseye)"' 'VERSION_ID="11"' 'ID=debian' ;;
        ubuntu2004) printf '%s\n' 'PRETTY_NAME="Ubuntu 20.04.6 LTS"' 'VERSION_ID="20.04"' 'ID=ubuntu' 'ID_LIKE=debian' ;;
        ubuntu2410) printf '%s\n' 'PRETTY_NAME="Ubuntu 24.10"' 'VERSION_ID="24.10"' 'ID=ubuntu' 'ID_LIKE=debian' ;;
        opensuse) printf '%s\n' 'NAME="openSUSE Leap"' 'VERSION_ID="15.6"' 'PRETTY_NAME="openSUSE Leap 15.6"' 'ID="opensuse-leap"' 'ID_LIKE="suse opensuse"' ;;
        sles) printf '%s\n' 'NAME="SLES"' 'VERSION_ID="15.6"' 'PRETTY_NAME="SUSE Linux Enterprise Server 15 SP6"' 'ID="sles"' 'ID_LIKE="suse"' ;;
        rocky9) printf '%s\n' 'NAME="Rocky Linux"' 'VERSION_ID="9.4"' 'PRETTY_NAME="Rocky Linux 9.4 (Blue Onyx)"' 'ID="rocky"' 'ID_LIKE="rhel centos fedora"' ;;
        fedora) printf '%s\n' 'NAME="Fedora Linux"' 'VERSION_ID=41' 'PRETTY_NAME="Fedora Linux 41 (Server Edition)"' 'ID=fedora' ;;
        # A derivative that claims Debian 12 through ID_LIKE only must still be refused.
        mint) printf '%s\n' 'PRETTY_NAME="Linux Mint 22"' 'VERSION_ID="22"' 'ID=linuxmint' 'ID_LIKE="ubuntu debian"' ;;
        # PRETTY_NAME with terminal escape bytes must not reach the message.
        escape) printf 'PRETTY_NAME="Evil \033[31mOS"\nVERSION_ID="1"\nID=evil\n' ;;
        noname) printf '%s\n' 'VERSION_ID="3"' 'ID=oddos' ;;
        *) die "unknown fixture: $name" ;;
    esac
}

passing="debian12 debian13 ubuntu2204 ubuntu2404 arch"
refused="kali debian11 ubuntu2004 ubuntu2410 opensuse sles rocky9 fedora mint escape noname"

# Run one gate in a fresh shell inside an empty directory. stdout+stderr go to
# $tmp/out; the exit status is returned.
run_install_gate() (
    cd "$work"
    # shellcheck disable=SC1090
    source "$install_defs"
    set +e
    require_supported_platform "$1"
)
run_get_gate() (
    cd "$work"
    shell=bash
    command -v dash >/dev/null 2>&1 && shell=dash
    "$shell" -c '
        set -eu
        . "$1"
        require_supported_platform "$2"
    ' gate "$get_defs" "$1"
)

assert_nothing_created() {
    local label=$1 listing
    listing=$(find "$work" -mindepth 1 | head -n 1)
    [[ -z "$listing" ]] || die "$label created files while checking the system: $listing"
}

check_message() {
    local label=$1 fixture=$2 output=$3
    grep -Fq 'This system is not supported:' <<< "$output" \
        || die "$label message has no English refusal for $fixture"
    grep -Fq 'Bu sistem desteklenmiyor:' <<< "$output" \
        || die "$label message has no Turkish refusal for $fixture"
    grep -Fq 'Nothing was changed on this server.' <<< "$output" \
        || die "$label message does not say nothing was changed ($fixture)"
    grep -Fq 'Bu sunucuda hiçbir şey değiştirilmedi.' <<< "$output" \
        || die "$label Turkish message does not say nothing was changed ($fixture)"
    grep -Fq 'Debian 12 and 13, Ubuntu 22.04 and 24.04, and Arch Linux' <<< "$output" \
        || die "$label message does not list the supported systems ($fixture)"
    grep -Fq 'https://celikpanel.net/systems.html' <<< "$output" \
        || die "$label message has no link to the supported list ($fixture)"
    if grep -q $'\033\\[31m' <<< "$output"; then
        die "$label message carried a terminal escape from os-release ($fixture)"
    fi
}

for gate in install get; do
    for fixture in $passing; do
        write_release "$fixture" > "$tmp/os-release"
        status=0
        "run_${gate}_gate" "$tmp/os-release" > "$tmp/out" 2>&1 || status=$?
        [[ "$status" -eq 0 ]] || die "$gate gate refused supported system $fixture: $(cat "$tmp/out")"
        [[ ! -s "$tmp/out" ]] || die "$gate gate printed text for supported system $fixture"
        assert_nothing_created "$gate gate ($fixture)"
    done
    for fixture in $refused; do
        write_release "$fixture" > "$tmp/os-release"
        status=0
        "run_${gate}_gate" "$tmp/os-release" > "$tmp/out" 2>&1 || status=$?
        [[ "$status" -ne 0 ]] || die "$gate gate accepted unsupported system $fixture"
        check_message "$gate gate" "$fixture" "$(cat "$tmp/out")"
        assert_nothing_created "$gate gate ($fixture)"
    done

    # The message names the detected system.
    write_release kali > "$tmp/os-release"
    "run_${gate}_gate" "$tmp/os-release" > "$tmp/out" 2>&1 || true
    grep -Fq 'not supported: Kali GNU/Linux Rolling.' "$tmp/out" \
        || die "$gate gate message does not name Kali: $(cat "$tmp/out")"
    write_release noname > "$tmp/os-release"
    "run_${gate}_gate" "$tmp/os-release" > "$tmp/out" 2>&1 || true
    grep -Fq 'not supported: oddos 3.' "$tmp/out" \
        || die "$gate gate message does not fall back to ID and version: $(cat "$tmp/out")"
done

# A missing os-release is also refused with the clear message by the public
# entry (get.sh): nothing identifies the system.
status=0
run_get_gate "$tmp/no-such-os-release" > "$tmp/out" 2>&1 || status=$?
[[ "$status" -ne 0 ]] || die "get.sh gate accepted a missing os-release"
grep -Fq 'not supported: unknown.' "$tmp/out" \
    || die "get.sh gate message for a missing os-release is unclear: $(cat "$tmp/out")"

# The gate must run before anything is changed. Static order checks.
function_body() { extract_function_source "$1" "$2"; }

function_body "$INSTALL" preflight_bootstrap_platform > "$tmp/preflight.txt"
gate_line=$(grep -n 'require_supported_platform "$os_release"' "$tmp/preflight.txt" | head -n 1 | cut -d: -f1)
classify_line=$(grep -n 'classify_bootstrap_platform "$os_release"' "$tmp/preflight.txt" | head -n 1 | cut -d: -f1)
[[ -n "$gate_line" && -n "$classify_line" && "$gate_line" -lt "$classify_line" ]] \
    || die "install.sh preflight must run the supported-system gate before classification"

# The installer runs the preflight before any package, user or file change.
preflight_call=$(grep -n '^preflight_bootstrap_platform "\$SELINUX_OS_RELEASE"' "$INSTALL" | head -n 1 | cut -d: -f1)
first_change=$(grep -n -E '^(useradd |install -d|mkdir |"\$APT_GET_BIN"|"\$PACMAN_BIN")' "$INSTALL" | head -n 1 | cut -d: -f1)
[[ -n "$preflight_call" && -n "$first_change" && "$preflight_call" -lt "$first_change" ]] \
    || die "install.sh must run the platform preflight before its first host change"

# get.sh: gate call sits before the first step that creates files, locks or records.
gate_call=$(grep -n '^\[ "\$operation" != install \] || require_supported_platform /etc/os-release$' "$GET" | cut -d: -f1)
storage_call=$(grep -n '^prepare_release_storage$' "$GET" | cut -d: -f1)
lock_call=$(grep -nE '^ +provision_first_install_signed_update_lock [|][|]' "$GET" | head -n 1 | cut -d: -f1)
pending_call=$(grep -n 'publish_pending_first_install ||' "$GET" | head -n 1 | cut -d: -f1)
[[ -n "$gate_call" && -n "$storage_call" && "$gate_call" -lt "$storage_call" ]] \
    || die "get.sh must run the supported-system gate before prepare_release_storage"
[[ -n "$lock_call" && "$gate_call" -lt "$lock_call" ]] \
    || die "get.sh must run the supported-system gate before the first-install lock"
[[ -n "$pending_call" && "$gate_call" -lt "$pending_call" ]] \
    || die "get.sh must run the supported-system gate before the pending install record"

# Derivatives are refused by ID, never admitted through ID_LIKE.
for file_function in "$INSTALL:require_supported_platform" "$GET:require_supported_platform"; do
    function_body "${file_function%%:*}" "${file_function##*:}" > "$tmp/gate-body.txt"
    ! grep -q 'ID_LIKE' "$tmp/gate-body.txt" \
        || die "${file_function%%:*} gate must not use ID_LIKE"
done

echo "unsupported platform gate test passed"
