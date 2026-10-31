#!/bin/bash
# Builds the disposable Debian 13 chroot for the D-040 real-system run:
# certbot, nginx and the Pebble test ACME server from Debian's own packages.
# Run as root on a development machine (never on an installed server).
# Usage: build-chroot.sh <chroot-dir>
#
# D-040 gerçek sistem denemesi için atılabilir Debian 13 chroot'u kurar.
set -euo pipefail
root=${1:?chroot directory required}
case "$root" in
  /var/tmp/*) ;;
  *) echo "refusing a chroot outside /var/tmp: $root" >&2; exit 2 ;;
esac
if [ -e "$root" ]; then
  echo "refusing to reuse an existing directory: $root" >&2
  exit 2
fi
mmdebstrap --variant=apt \
  --include=certbot,nginx-light,pebble,ca-certificates,openssl,iproute2,procps,curl,python3,postfix,dovecot-imapd \
  trixie "$root" http://deb.debian.org/debian
# Marker the test binary insists on before it touches anything.
echo "disposable D-040 chroot built $(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$root/etc/celikpanel-sitecert-chroot"
cat >"$root/etc/hosts" <<'EOF'
127.0.0.1 localhost site.test www.site.test mail.site.test mx.site.test
::1 localhost
EOF
# nginx's packaged default site would answer every name on port 80.
rm -f "$root/etc/nginx/sites-enabled/default"
chroot "$root" dpkg-query -W certbot nginx-light pebble python3-acme openssl postfix dovecot-core dovecot-imapd >"$root/root/package-versions.txt"
cat "$root/root/package-versions.txt"
