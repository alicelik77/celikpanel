# guest (root, read-only): hashes of the native web/PHP/mail configuration. $1 = label; writes /root/snap-$1.txt
# and, with $2 = an earlier label, prints the difference. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
L=$1; O=/root/snap-$L.txt
{
  for d in /etc/nginx /etc/php /etc/postfix /etc/dovecot /etc/systemd/system /etc/postfwd /etc/default/postfwd; do
    [ -e "$d" ] && find "$d" \( -type f -o -type l \) 2>/dev/null | sort | while read -r f; do
      if [ -L "$f" ]; then echo "link:$(readlink "$f") $f"; else echo "$(sha256sum "$f" | cut -c1-16) $f"; fi
    done
  done
  echo "postconf-n $(postconf -n 2>/dev/null | sha256sum | cut -c1-16)"
  echo "postconf-M $(postconf -M 2>/dev/null | sha256sum | cut -c1-16)"
  echo "postconf-P $(postconf -P 2>/dev/null | sha256sum | cut -c1-16)"
} > $O
echo "snapshot $L: $(wc -l < $O) entries"
if [ -n "${2:-}" ]; then
  echo "--- differences $2 -> $L (excluding systemd self-update/runtime units)"
  diff /root/snap-$2.txt $O | grep -E '^[<>]' | grep -vE 'celikpanel-self-update|/etc/systemd/system/(multi-user|timers|sockets)\.target\.wants' | head -80
  echo "(end)"
  postconf -P 2>/dev/null | grep -iE "policy|end_of_data" ; echo "(postconf -P policy lines end)"
fi
