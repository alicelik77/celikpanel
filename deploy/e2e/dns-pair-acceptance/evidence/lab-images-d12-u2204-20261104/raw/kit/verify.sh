#!/bin/bash
# Re-verify the two new lab images against the distributions' published sums. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
set -u
I=/var/tmp/cp-v3n28/images
D=$(mktemp -d /var/tmp/cp-imgverify.XXXXXX); cd "$D" || exit 1
echo "== Debian 12 bookworm 20261026-2643"
curl -fsS --proto =https -o d12.SHA512SUMS https://cloud.debian.org/images/cloud/bookworm/20261026-2643/SHA512SUMS && echo "sums fetched"
for s in SHA512SUMS.sign SHA512SUMS.gpg; do curl -fsS -o /dev/null -w "$s HTTP %{http_code}\n" https://cloud.debian.org/images/cloud/bookworm/20261026-2643/$s 2>&1 | tail -1; done
pub=$(awk '$2=="debian-12-genericcloud-amd64-20261026-2643.qcow2"{print $1}' d12.SHA512SUMS)
loc=$(sha512sum $I/debian-12-genericcloud-amd64-20261026-2643.qcow2 | awk '{print $1}')
echo "published $pub"; echo "local     $loc"; [ -n "$pub" ] && [ "$pub" = "$loc" ] && echo "DEBIAN12 SHA512 MATCH" || echo "DEBIAN12 MISMATCH"
echo "== Ubuntu 22.04 jammy release-20261020"
U=https://cloud-images.ubuntu.com/releases/jammy/release-20261020
curl -fsS --proto =https -o SHA256SUMS $U/SHA256SUMS && curl -fsS --proto =https -o SHA256SUMS.gpg $U/SHA256SUMS.gpg && echo "sums+sig fetched"
export GNUPGHOME=$D/gnupg; mkdir -m 700 $GNUPGHOME
gpg -q --keyserver hkps://keyserver.ubuntu.com --recv-keys D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81 2>&1 | tail -2
gpg --fingerprint D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81 2>&1 | sed -n 2,3p
gpg --status-fd 1 --verify SHA256SUMS.gpg SHA256SUMS 2>/dev/null | grep -E 'VALIDSIG|GOODSIG|BADSIG|ERRSIG' | cut -c1-120
pub=$(awk '$2=="*ubuntu-22.04-server-cloudimg-amd64.img"||$2=="ubuntu-22.04-server-cloudimg-amd64.img"{print $1}' SHA256SUMS)
loc=$(sha256sum $I/ubuntu-22.04-server-cloudimg-amd64-20261020.img | awk '{print $1}')
echo "published $pub"; echo "local     $loc"; [ -n "$pub" ] && [ "$pub" = "$loc" ] && echo "UBUNTU2204 SHA256 MATCH" || echo "UBUNTU2204 MISMATCH"
echo "== qemu-img info"
for f in debian-12-genericcloud-amd64-20261026-2643.qcow2 ubuntu-22.04-server-cloudimg-amd64-20261020.img; do qemu-img info $I/$f | grep -E 'file format|virtual size|backing'; done
cd /; rm -rf "$D"; echo "verify dir removed"
