#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
builder="$repo_root/deploy/build-download-portal.sh"
bootstrap="$repo_root/download-portal/get.sh"

fail() {
  printf 'download portal contract failed: %s\n' "$1" >&2
  exit 1
}

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT HUP INT TERM

version=v1.2.3-alpha.4
commit=0123456789abcdef0123456789abcdef01234567
published_at=2026-08-03T08:55:11Z
archive="celikpanel-$version.tar.gz"
mkdir -p "$tmp/source/celikpanel-$version"
printf '#!/bin/sh\nprintf test-install\\n\n' > "$tmp/source/celikpanel-$version/install.sh"
tar -czf "$tmp/$archive" -C "$tmp/source" "celikpanel-$version"
(cd "$tmp" && sha256sum "$archive" > "$archive.sha256")

bash "$builder" "$version" "$commit" "$published_at" \
  "$tmp/$archive" "$tmp/$archive.sha256" "$tmp/site"

# The public site: every page and asset of the generator's reviewed list is
# shipped, and a page is the reviewed page with only its release placeholders
# filled. The list itself is not published.
site_manifest="$repo_root/download-portal/site.sha256"
[[ -s "$site_manifest" ]] || fail "the site manifest is missing"
[[ ! -e "$tmp/site/site.sha256" ]] || fail "the site manifest was published"
while read -r digest file; do
  [[ -s "$tmp/site/$file" ]] || fail "site file was not shipped: $file"
  case "$file" in
    *.html) ;;
    *) cmp -s -- "$repo_root/download-portal/$file" "$tmp/site/$file" \
         || fail "site asset bytes changed: $file" ;;
  esac
done < "$site_manifest"
for page in index.html pricing.html download.html systems.html releases.html technical.html \
  tr/index.html tr/fiyat.html tr/indir.html tr/sistemler.html tr/surumler.html; do
  grep -Fq "  $page" "$site_manifest" || fail "site page is not in the manifest: $page"
  ! grep -Eq '@@CP_|cp:/?signed' "$tmp/site/$page" || fail "release placeholder left in: $page"
  ! grep -Eq '<script>|<script [^>]*>[^<]' "$tmp/site/$page" || fail "inline script in: $page"
  ! grep -Eq ' style="|<style' "$tmp/site/$page" || fail "inline style in: $page"
  ! grep -Eq '(src|href)="(https?:)?//[^"]*\.(css|js|woff2|webp|png|svg)"' "$tmp/site/$page" \
    || fail "page loads an outside resource: $page"
done
# The pages are the reviewed pages with only the placeholders filled: put the
# placeholders back and the bytes must be the committed ones (the unsigned
# test release drops the signed block, so that page is compared without it).
python3 - "$repo_root/download-portal" "$tmp/site" "$version" "$commit" <<'PY'
import pathlib, re, sys
source, built, version, commit = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), sys.argv[3], sys.argv[4]
for line in (source / "site.sha256").read_text(encoding="utf-8").splitlines():
    name = line.split()[1]
    if not name.endswith(".html"):
        continue
    reviewed = (source / name).read_text(encoding="utf-8")
    reviewed = re.sub(r"<!-- cp:signed -->\n.*?<!-- cp:/signed -->\n", "", reviewed, flags=re.S)
    shipped = (built / name).read_text(encoding="utf-8")
    pattern = re.escape(reviewed)
    pattern = re.sub(r"@@CP_[A-Z0-9_]+@@", lambda m: "[^<>\"]+", pattern)
    assert re.fullmatch(pattern, shipped, flags=re.S), f"shipped page differs beyond its placeholders: {name}"
    if "@@CP_RELEASE_VERSION@@" in reviewed:
        assert version in shipped, name
PY
grep -Fq 'v1.2.3-alpha.4' "$tmp/site/releases.html" || fail "releases page does not name the release"
grep -Fq '3 August 2026' "$tmp/site/releases.html" || fail "releases page does not date the release in English"
grep -Fq '3 Ağustos 2026' "$tmp/site/tr/surumler.html" || fail "releases page does not date the release in Turkish"
grep -Fq "/releases/$version/$archive" "$tmp/site/releases.html" || fail "releases page does not link the archive"
! grep -Fq '.sig"' "$tmp/site/releases.html" || fail "unsigned release links a signature"
# The supported-systems page keeps its address and names every supported
# system, what Arch does not offer and how updates work, in both languages;
# the other pages link to it.
for needle in 'Debian 13 <span class="code-name">trixie' 'Debian 12 <span class="code-name">bookworm' \
  'Ubuntu 24.04 LTS <span class="code-name">noble' 'Ubuntu 22.04 LTS <span class="code-name">jammy' 'Arch Linux' \
  'sudo pacman -Syu' 'archlinux.org/news' 'phpPgAdmin' 'SpamAssassin' 'Valkey' 'unattended-upgrades' \
  'Supported operating systems' 'How updates work' 'is a separate program and not an update'; do
  grep -Fq -- "$needle" "$tmp/site/systems.html" || fail "systems page lacks: $needle"
done
for needle in 'Desteklenen işletim sistemleri' 'Güncellemeler nasıl işler' 'ayrı bir programdır, güncelleme değildir' 'sudo pacman -Syu'; do
  grep -Fq -- "$needle" "$tmp/site/tr/sistemler.html" || fail "Turkish systems page lacks: $needle"
done
for page in index.html pricing.html download.html releases.html; do
  grep -Fq 'href="systems.html"' "$tmp/site/$page" || fail "portal page does not link to the supported systems: $page"
done
grep -Fq 'href="sistemler.html"' "$tmp/site/tr/index.html" || fail "Turkish home does not link to the supported systems"
# The old technical page points to its new place in both languages.
grep -Fq 'href="download.html#how"' "$tmp/site/technical.html" || fail "technical.html does not point to the download page"
grep -Fq 'href="tr/indir.html#nasil"' "$tmp/site/technical.html" || fail "technical.html does not point to the Turkish download page"
# alpha.119: the user guide. Every page of its reviewed manifest is shipped
# byte for byte, in both languages, without script; the manifest itself is
# not; the other pages link to the guide in the visitor's language.
guide_manifest="$repo_root/download-portal/guide/manifest.sha256"
[[ -s "$guide_manifest" ]] || fail "the guide manifest is missing"
(cd "$tmp/site/guide" && sha256sum --quiet --strict -c "$guide_manifest") >/dev/null 2>&1 \
  || fail "guide pages were not shipped exactly"
[[ ! -e "$tmp/site/guide/manifest.sha256" ]] || fail "the guide manifest was published"
[[ "$(cd "$tmp/site/guide" && find . -type f | wc -l)" -eq "$(wc -l < "$guide_manifest")" ]] \
  || fail "the guide has pages its manifest does not list"
for page in index.html en/index.html tr/index.html en/getting-started.html tr/getting-started.html \
  en/reading-states.html tr/reading-states.html en/files-access.html tr/files-access.html; do
  [[ -s "$tmp/site/guide/$page" ]] || fail "guide page was not shipped: $page"
  ! grep -Fq '<script' "$tmp/site/guide/$page" || fail "guide page carries a script: $page"
  grep -Fq 'href="/assets/guide.css"' "$tmp/site/guide/$page" || fail "guide page lacks its stylesheet: $page"
done
grep -Fq '<html lang="tr">' "$tmp/site/guide/tr/files-access.html" || fail "Turkish guide page is not marked Turkish"
grep -Fq '<html lang="en">' "$tmp/site/guide/en/files-access.html" || fail "English guide page is not marked English"
for page in index.html pricing.html download.html systems.html releases.html; do
  grep -Fq 'href="guide/en/"' "$tmp/site/$page" || fail "portal page does not link to the guide: $page"
done
grep -Fq 'href="../guide/tr/"' "$tmp/site/tr/index.html" || fail "Turkish home does not link to the Turkish guide"
for asset in public.css home.css pages.css public.js boot.js site.css guide.css favicon-v2.svg \
  fonts/overpass-latin.woff2 fonts/overpass-latin-ext.woff2 fonts/overpass-mono-latin.woff2 fonts/overpass-mono-latin-ext.woff2 \
  fonts/OFL-Overpass.txt fonts/OFL-OverpassMono.txt \
  shots/domains-en-light-2x.webp shots/domain-en-dark-phone.webp shots/domain-tr-light.webp shots/databases-tr-dark-2x.webp \
  shots/domains-en-light.webp shots/domains-tr-dark.webp shots/databases-en-light.webp shots/databases-tr-dark.webp; do
  [[ -s "$tmp/site/assets/$asset" ]] || fail "required asset was not shipped: $asset"
  cmp -s -- "$repo_root/download-portal/assets/$asset" "$tmp/site/assets/$asset" \
    || fail "portal asset bytes changed: $asset"
done
[[ -f "$tmp/site/.well-known/security.txt" ]] || fail "security.txt was not generated"
[[ -x "$tmp/site/get.sh" ]] || fail "bootstrap is not executable"
[[ -f "$tmp/site/release-signing-ed25519.pem" ]] || fail "release public key was not generated"
[[ "$(sha256sum "$tmp/site/release-signing-ed25519.pem" | awk '{print $1}')" == \
   7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb ]] \
  || fail "release public key digest is wrong"
[[ -f "$tmp/site/releases/$version/$archive" ]] || fail "versioned archive is missing"
[[ "$(cat "$tmp/site/releases/latest.txt")" == "$version" ]] || fail "latest pointer is wrong"
cmp "$tmp/$archive" "$tmp/site/releases/$version/$archive" || fail "archive bytes changed"

python3 - "$tmp/site/releases/latest.json" "$tmp/site/releases/index.json" <<'PY'
import json
import pathlib
import sys

latest = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
index = json.loads(pathlib.Path(sys.argv[2]).read_text(encoding="utf-8"))
assert latest["version"] == "v1.2.3-alpha.4"
assert latest["commit"] == "0123456789abcdef0123456789abcdef01234567"
assert len(latest["sha256"]) == 64
assert index["latest"] == latest["version"]
assert index["releases"] == [latest]
PY

grep -Fq "Options -Indexes" "$tmp/site/.htaccess" || fail "directory listing is not disabled"
grep -Fq "Content-Security-Policy" "$tmp/site/.htaccess" || fail "CSP header is missing"
# Language: each page links its other-language page, marks the current one
# and says which language each link opens; the choice is remembered only by
# the small progressive script.
python3 - "$tmp/site" <<'PY'
from html.parser import HTMLParser
import pathlib
import sys

site = pathlib.Path(sys.argv[1])
pairs = {"index.html": "tr/index.html", "pricing.html": "tr/fiyat.html", "download.html": "tr/indir.html",
         "systems.html": "tr/sistemler.html", "releases.html": "tr/surumler.html"}

class Page(HTMLParser):
    def __init__(self):
        super().__init__()
        self.lang = None
        self.alternates = {}
        self.switch = []
    def handle_starttag(self, tag, attrs):
        values = dict(attrs)
        if tag == "html":
            self.lang = values.get("lang")
        if tag == "link" and values.get("rel") == "alternate":
            self.alternates[values.get("hreflang")] = values.get("href")
        if tag == "a" and "data-set-lang" in values:
            self.switch.append(values)

for en, tr in pairs.items():
    for name, lang, other in ((en, "en", "tr"), (tr, "tr", "en")):
        page = Page()
        page.feed((site / name).read_text(encoding="utf-8"))
        assert page.lang == lang, (name, page.lang)
        assert set(page.alternates) == {"en", "tr", "x-default"}, (name, page.alternates)
        assert len(page.switch) == 1 and page.switch[0]["data-set-lang"] == other, (name, page.switch)
        assert page.switch[0]["lang"] == other and page.switch[0]["hreflang"] == other, name
        assert page.switch[0]["aria-label"] == ("Türkçe" if other == "tr" else "English"), name
        target = (site / name).parent / page.switch[0]["href"]
        target = target / "index.html" if page.switch[0]["href"].endswith("/") else target
        assert target.resolve() == (site / (tr if lang == "en" else en)).resolve(), (name, page.switch[0]["href"])
PY
grep -Fq 'celikpanel-language' "$tmp/site/assets/public.js" || fail "language preference persistence is missing"
grep -Fq 'celikpanel-language' "$tmp/site/assets/boot.js" || fail "remembered language is not applied"
grep -Fq 'A hosting control panel for Linux servers' "$tmp/site/index.html" || fail "English product copy is missing"
grep -Fq 'Linux sunucular için hosting kontrol paneli' "$tmp/site/tr/index.html" || fail "Turkish product copy is missing"
[[ -f "$tmp/site/assets/fonts/overpass-latin-ext.woff2" ]] \
  || fail "self-hosted typeface with Turkish coverage was not shipped"
[[ -f "$tmp/site/assets/fonts/OFL-Overpass.txt" ]] \
  || fail "typeface licence was not shipped"
grep -Fq "font-src 'self'" "$tmp/site/.htaccess" \
  || fail "CSP does not allow the self-hosted typefaces"
grep -Fq "script-src 'self'" "$tmp/site/.htaccess" || fail "CSP does not keep scripts to the site"
grep -Fq "style-src 'self'" "$tmp/site/.htaccess" || fail "CSP does not keep styles to the site"
grep -Fq 'Cache-Control "no-cache, must-revalidate"' "$tmp/site/.htaccess" \
  || fail "the page and its stylesheet may be served stale from cache"
for lang in en tr; do
  page=$([[ $lang == en ]] && echo index.html || echo tr/index.html)
  grep -Fq "alicelik77/celikpanel-feedback/issues/new?template=bug_$lang.yml" "$tmp/site/$page" \
    || fail "bug report link is missing ($lang)"
  grep -Fq "alicelik77/celikpanel-feedback/issues/new?template=feature_$lang.yml" "$tmp/site/$page" \
    || fail "feature request link is missing ($lang)"
  grep -Fq 'alicelik77/celikpanel-feedback/security/advisories/new' "$tmp/site/$page" \
    || fail "private security report link is missing ($lang)"
  grep -Fq "account/?action=register&amp;lang=$lang" "$tmp/site/$page" \
    || fail "create-account link is missing ($lang)"
  grep -Fq "account/?action=login&amp;lang=$lang" "$tmp/site/$page" \
    || fail "sign-in link is missing ($lang)"
done
# Nothing can be bought before 1.0: the price is shown as the 1.0 price and
# no page offers a purchase or a trial that does not exist yet.
grep -Fq 'from version 1.0' "$tmp/site/pricing.html" || fail "pricing page does not say the price starts with 1.0"
grep -Fq '1.0 sürümünden itibaren' "$tmp/site/tr/fiyat.html" || fail "Turkish pricing page does not say the price starts with 1.0"
for page in index.html pricing.html tr/index.html tr/fiyat.html; do
  ! grep -Eiq '<(a|button)[^>]*>[^<]*(buy|satın al|start a (30-day )?trial|denemeyi başlat|deneme başlat)' "$tmp/site/$page" \
    || fail "page offers a purchase or trial that does not exist yet: $page"
done
grep -Fq 'Canonical: https://celikpanel.net/.well-known/security.txt' "$tmp/site/.well-known/security.txt" \
  || fail "security.txt canonical URL is missing"
grep -Fq 'Preferred-Languages: tr, en' "$tmp/site/.well-known/security.txt" \
  || fail "security.txt language preference is missing"
if grep -Eq '<button[^>]*>(\+ Yeni site|×|Devam et)' "$tmp/site/index.html"; then
  fail "illustrative product controls must not enter the keyboard tab order"
fi
grep -Fq -- "--proto '=https'" "$bootstrap" || fail "HTTPS protocol restriction is missing"
grep -Fxq 'bootstrap_release_sequence=145' "$bootstrap" || fail "bootstrap sequence pin is missing"
grep -Fxq 'bootstrap_release_version=v0.1.0-alpha.145' "$bootstrap" || fail "bootstrap version pin is missing"
grep -Fxq 'bootstrap_release_public_key_sha256=7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb' "$bootstrap" \
  || fail "bootstrap public-key pin is missing"
grep -Fq "sha256sum -c" "$bootstrap" || fail "archive checksum is not verified"
grep -Fq "exact internal checksum manifest" "$bootstrap" || fail "exact internal manifest validation is missing"
grep -Fq "release.commit release.tree" "$bootstrap" || fail "release provenance validation is missing"
grep -Fq "Archive contains unsafe or unexpected paths" "$bootstrap" || fail "path validation is missing"
grep -Fq "Arşiv güvensiz veya beklenmeyen yollar içeriyor" "$bootstrap" || fail "Turkish path validation is missing"
grep -Fq 'for required_command in awk bash chmod chown cmp curl dirname env find flock grep id install' "$bootstrap" \
  || fail "complete runtime requirement gate is missing"
grep -Fq 'bash "$installer"' "$bootstrap" || fail "installer is not executed with bash"
grep -Fq -- '--install|--update' "$bootstrap" || fail "install/update selector is missing"
grep -Fq '/etc/celikpanel/install.complete' "$bootstrap" || fail "completion marker gate is missing"
grep -Fq 'A partial or ambiguous CelikPanel installation was found.' "$bootstrap" \
  || fail "partial installation refusal is missing"
grep -Fq '# BEGIN DOWNLOAD OPERATION POLICY' "$bootstrap" \
  || fail "extractable download operation policy is missing"
grep -Fq 'detect_known_interrupted_update_candidate_at()' "$bootstrap" \
  || fail "interrupted-update detector is missing"
grep -Fq '/var/lib/celikpanel-release-transaction' "$bootstrap" \
  || fail "fixed production transaction root is missing"
grep -Fq '8bbbac8b628fae4fca0e127e52c1c7835f56f8b8' "$bootstrap" \
  || fail "known interrupted alpha.4 target is missing"
grep -Fq 'select_download_operation()' "$bootstrap" \
  || fail "pure download operation selector is missing"
grep -Fq 'recovery-update' "$bootstrap" \
  || fail "narrow recovery-update classification is missing"
[[ "$(grep -Fc 'detect_known_interrupted_update_candidate || fail' "$bootstrap")" -eq 2 ]] \
  || fail "interrupted-update evidence is not rechecked exactly twice"
grep -Fq 'bootstrap-prebuilt-update.sh' "$bootstrap" || fail "prebuilt updater is missing"
grep -Fq 'Updating CelikPanel to' "$bootstrap" || fail "English update progress is missing"
grep -Fq "Usage:" "$bootstrap" || fail "English usage text is missing"
grep -Fq "Kullanım:" "$bootstrap" || fail "Turkish usage text is missing"
grep -Fq "Installing CelikPanel" "$bootstrap" || fail "English install progress is missing"
grep -Fq "doğrulanmış" "$bootstrap" || fail "Turkish install progress is missing"
for page in index.html download.html tr/index.html tr/indir.html; do
  if grep -Eq 'curl[^\n]*\|[[:space:]]*(ba)?sh' "$repo_root/download-portal/$page"; then
    fail "page recommends curl-pipe-shell: $page"
  fi
done

if bash "$builder" "$version" "$commit" "$published_at" \
  "$tmp/$archive" "$tmp/$archive.sha256" "$tmp/site" >/dev/null 2>&1; then
  fail "builder overwrote an existing output directory"
fi

printf 'download portal contract passed\n'
