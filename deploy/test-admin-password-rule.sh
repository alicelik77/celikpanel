#!/bin/bash
# The first administrator's password meets the owner's rule (6 Nov 2026) up
# front: get.sh checks the password in CELIKPANEL_ADMIN_CREDENTIALS_FILE
# before anything is downloaded or changed, names the rule that is not met in
# English and Turkish, and the rule is the panel's own (internal/passwordpolicy,
# field "account": 8 to 1024 characters, an uppercase letter, a lowercase
# letter and a special character). The terminal prompt (panel --create-admin)
# asks again on a weak password; that is tested in cmd/panel.
#
# İlk yöneticinin parolası sahibin kuralına baştan uyar: get.sh dosyadaki
# parolayı hiçbir şey indirilmeden denetler ve eksik kuralı iki dilde söyler.
#
# Needs only bash, awk (and dash when present). It reads fixture files only.
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
GET="$ROOT/download-portal/get.sh"
FIELDS="$ROOT/internal/passwordpolicy/testdata/fields.json"

die() {
    echo "admin password rule test failed: $*" >&2
    exit 1
}

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

defs=$tmp/get-defs.sh
{
    for name in message fail validate_admin_credentials_content_subset admin_password_rule_refusal; do
        extract_function_source "$GET" "$name" || die "get.sh has no complete function: $name"
    done
} > "$defs"

# The entry's window is the panel's account window.
grep -Eq '"account": \{ "min": 8, "max": 1024 \}' "$FIELDS" \
    || die "internal/passwordpolicy account window is no longer 8..1024; update get.sh with it"

shells=(bash)
command -v dash >/dev/null 2>&1 && shells+=(dash)

# file content -> expected exit status of the subset check
check() {
    local label=$1 password=$2 want=$3 shell status
    printf '{"username":"first-admin","email":"admin@example.test","password":"%s"}\n' "$password" > "$tmp/credentials"
    for shell in "${shells[@]}"; do
        status=0
        "$shell" -c ". '$defs'; validate_admin_credentials_content_subset '$tmp/credentials'" || status=$?
        [[ $status == "$want" ]] || die "$label under $shell: status $status, want $want"
    done
}

check compliant 'Admin-pass1' 0
check compliant-no-digit 'Admin-pass' 0
check compliant-space 'Admin pass' 0
check too-short 'Ab-1' 3
check no-uppercase 'abcdefgh-1' 4
check no-lowercase 'ABCDEFGH-1' 5
check no-special 'Abcdefgh1' 6
check too-long "Ab-$(printf 'x%.0s' $(seq 1 1022))" 7
check form-quote-free 'Ab"cdefgh' 1

# The refusal names the rule in both languages, says nothing was changed and
# never prints the password.
for code in 3 4 5 6 7; do
    for shell in "${shells[@]}"; do
        out=$("$shell" -c ". '$defs'; admin_password_rule_refusal $code" 2>&1) && die "refusal $code did not stop under $shell"
        case "$code" in
            3) en='at least 8 characters'; tr='en az 8 karakter' ;;
            4) en='an uppercase letter'; tr='büyük harf' ;;
            5) en='a lowercase letter'; tr='küçük harf' ;;
            6) en='a special character'; tr='özel karakter' ;;
            7) en='at most 1024 characters'; tr='en fazla 1024 karakter' ;;
        esac
        [[ $out == *"$en"* && $out == *"$tr"* ]] || die "refusal $code under $shell does not name the rule: $out"
        [[ $out == *'Nothing was downloaded or changed'* && $out == *'hiçbir şey indirilmedi'* ]] \
            || die "refusal $code under $shell does not say nothing changed: $out"
        [[ $out != *'Admin-pass1'* ]] || die "refusal printed a password"
    done
done

# Both languages exist for every rule.
for needle in 'must be at least 8 characters' 'en az 8 karakter olmalı' 'must contain an uppercase letter' \
    'en az bir büyük harf içermeli' 'must contain a lowercase letter' 'en az bir küçük harf içermeli' \
    'must contain a special character' 'en az bir özel karakter'; do
    grep -Fq "$needle" "$GET" || die "get.sh lacks: $needle"
done

# The check runs before the download: the refusal is inside the first
# administrator's admission, which get.sh runs before it fetches anything.
grep -Fq 'admin_password_rule_refusal "$admission_subset_status"' "$GET" || die "the admission does not use the rule refusal"

echo "admin password rule test passed"
