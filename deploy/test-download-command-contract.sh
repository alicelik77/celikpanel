#!/usr/bin/env bash
set -euo pipefail
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
portal=$repo_root/download-portal
fail() { printf 'download command contract failed: %s\n' "$1" >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
# The public site shows one install command, generated from one template
# (site/templates/parts.html). Every page that shows it shows the same three
# lines, with its own Copy button that copies exactly that text.
python3 - "$portal" > "$tmp/command" <<'PY'
import html, pathlib, re, sys
portal = pathlib.Path(sys.argv[1])
commands = set()
for name, words in (("index.html", "root terminal"), ("download.html", "root terminal"),
                    ("tr/index.html", "root terminalinde"), ("tr/indir.html", "root terminalinde")):
    text = (portal / name).read_text(encoding="utf-8")
    found = re.findall(r'<code id="(cmd-[a-z]+)">(.*?)</code>', text, re.S)
    assert len(found) == 1, (name, len(found))
    ident, body = found[0]
    assert text.count(f'data-copy="{ident}"') == 1, name
    assert 'exact-command' not in text and 'command-disclosure' not in text, name
    assert words in text, name
    commands.add(html.unescape(body))
assert len(commands) == 1, "pages show different install commands"
print(commands.pop())
PY
dash -n "$tmp/command"
[[ $(wc -l < "$tmp/command") == 3 ]] || fail 'public command is not three lines'
[[ $(head -1 "$tmp/command") == 'timedatectl set-ntp true' ]] || fail 'public command does not start with the clock line'
grep -Fq 'navigator.clipboard.writeText(source.textContent' "$portal/assets/public.js" \
  || fail 'Copy does not copy the visible command text'
mkdir "$tmp/bin"
cat > "$tmp/bin/curl" <<'SH'
#!/bin/sh
printf '%s\n' "$@" > "$TEST_ROOT/curl-args"
if [ "${TEST_FAIL:-0}" = 1 ]; then
  printf '%s\n' partial-download > celikpanel-install.sh
  exit 22
fi
printf '%s\n' downloaded-bootstrap > celikpanel-install.sh
SH
cat > "$tmp/bin/sh" <<'SH'
#!/bin/sh
[ "$#" = 1 ] && [ "$1" = celikpanel-install.sh ] || exit 2
[ "$(cat "$1")" = downloaded-bootstrap ] || exit 3
touch "$TEST_ROOT/executed"
SH
cat > "$tmp/bin/timedatectl" <<'SH'
#!/bin/sh
printf '%s
' "$*" > "$TEST_ROOT/timedatectl-args"
[ "${TEST_NTP_FAIL:-0}" = 1 ] && exit 1
exit 0
SH
chmod +x "$tmp/bin/curl" "$tmp/bin/sh" "$tmp/bin/timedatectl"
(cd "$tmp"; TEST_ROOT=$tmp PATH=$tmp/bin:/usr/bin:/bin dash "$tmp/command")
[[ -f "$tmp/executed" ]] || fail 'successful download did not run'
for argument in -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh; do
  grep -Fxq -- "$argument" "$tmp/curl-args" || fail "missing curl argument: $argument"
done
[[ $(cat "$tmp/timedatectl-args") == 'set-ntp true' ]] || fail 'clock line did not run timedatectl set-ntp true'
rm "$tmp/executed"
# A host without NTP support still installs; the clock line only reports it.
(cd "$tmp"; TEST_ROOT=$tmp TEST_NTP_FAIL=1 PATH=$tmp/bin:/usr/bin:/bin dash "$tmp/command")
[[ -f "$tmp/executed" ]] || fail 'a failed clock line stopped the installation'
rm "$tmp/executed"
if (cd "$tmp"; TEST_ROOT=$tmp TEST_FAIL=1 PATH=$tmp/bin:/usr/bin:/bin dash "$tmp/command"); then
  fail 'download failure was not propagated'
fi
[[ ! -e "$tmp/executed" ]] || fail 'failed download executed a stale or partial file'
printf 'download command contract passed\n'
