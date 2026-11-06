# guest (root, read-only): the DNS check tool, named's control sockets, site include dirs. Lab only.
echo "release $(cat /opt/celikpanel/release.commit 2>/dev/null) nproc $(nproc)"
for f in /usr/local/libexec/celikpanel-bind-peer-inspect /usr/local/libexec/celikpanel-pdns-peer-inspect; do [ -e $f ] && echo "tool $(sha256sum $f | cut -c1-64) $(stat -c '%U:%G %a %s %y' $f) $f"; done
for f in /opt/celikpanel/dns-owner-tools/*; do [ -f "$f" ] && echo "owner-tools $(sha256sum $f | cut -c1-16) $f"; done
for d in /var/backups/celikpanel/releases/*/; do [ -f $d/dns-owner-tools/bind-peer-inspect ] && echo "release-tree $(basename $d) bind $(sha256sum $d/dns-owner-tools/bind-peer-inspect | cut -c1-16)"; done
echo "--- refresh record dir"; ls -la /var/lib/celikpanel-agent-private/dns-check-tool/ 2>&1 | tail -n +2
for f in /var/lib/celikpanel-agent-private/dns-check-tool/*.json* /var/lib/celikpanel-agent-private/dns-check-tool/*record*; do [ -f "$f" ] && { echo "== $f"; head -c 1500 "$f"; echo; }; done
echo "--- named control sockets (ss)"; command -v named >/dev/null && named -v; ss -ltnpH 'sport = :953' | sed 's/  */ /g'
echo "--- site include dirs"; ls -la /etc/nginx/celikpanel-sites.d/ 2>&1 | tail -n +2
[ -n "$1" ] && { echo "--- $1 traces"; ls -ld /etc/nginx/celikpanel-sites.d/$1 /etc/nginx/sites-enabled/*$1* /var/www/*$1* /home/*/$1 2>&1 | head; }
echo "--- agent journal: check tool"; journalctl -u celikpanel-agent --no-pager -o short-iso --since -3h | grep -iE "check tool|peer-inspect|dns-check-tool|inspector" | cut -c1-300 | tail -8
