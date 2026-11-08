# guest (root) on A: mailbox recovery probes. usage: g_rec.sh count USER | token USER OUTFILE | users | who USER | hashes. Lab only.
# The token is written to OUTFILE (0600) and never printed.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
case "$1" in
count) echo "messages in $2: $(doveadm search -u "$2" mailbox INBOX all 2>/dev/null | wc -l)"
       doveadm fetch -u "$2" 'date.received hdr.subject' mailbox INBOX all 2>/dev/null | grep -iE "^hdr.subject|^date" | tail -6 | cut -c1-160 ;;
token) uid=$(doveadm search -u "$2" mailbox INBOX all 2>/dev/null | tail -1 | awk '{print $2}')
       body=$(doveadm fetch -u "$2" 'body hdr.subject' mailbox INBOX uid "$uid" 2>/dev/null)
       echo "subject: $(echo "$body" | sed -n 's/^hdr.subject: //p' | head -1 | cut -c1-140)"
       url=$(echo "$body" | tr -d '\r' | sed ':a;/=$/{N;s/=\n//;ba}' | sed 's/=3D/=/g' | grep -oE 'https?://[^ <>"]*#t=[A-Za-z0-9_.~-]+' | head -1)
       p=${url%%#*}; echo "link: ${p}#t=[REDACTED]"
       tok=${url##*#t=}; [ "$tok" = "$url" ] && tok=
       umask 077; printf '%s' "$tok" > "$3"; echo "token length ${#tok}" ;;
users) awk -F: '{print $1, substr($2,1,12) "..."}' /etc/dovecot/users 2>/dev/null | head; sha256sum /etc/dovecot/users | cut -c1-16 ;;
hashes) awk -F: '{print $1}' /etc/dovecot/users | while read -r u; do printf '%s %s\n' "$u" "$(grep "^$u:" /etc/dovecot/users | sha256sum | cut -c1-12)"; done ;;
who) doveadm who 2>/dev/null | grep -E "^username|$2" | head -5 ;;
esac
