#!/bin/bash
# guest (root) on the main server after the update: case 4 sending paths. Lab only (sends mail as a site / root / vmail would).
# $1 site domain, $2 docroot, $3 site user, $4 own mailbox (local), $5 outside address (a mailbox on another lab server)
D=$1; DR=$2; SU=$3; LOCAL=$4; EXT=$5
echo "postconf: $(postconf authorized_submit_users smtpd_relay_restrictions | tr '\n' ';')"
printf '<?php var_dump(mail("%s", "a125 php mail() fpm", "sent by the site with PHP mail()", "From: wordpress@%s"));\n' "$EXT" "$D" > "$DR/a125-mail.php"; chown $SU: "$DR/a125-mail.php"
echo "--- 1a. PHP mail() from the site (PHP-FPM pool as $SU)"
curl -s -k -m 30 --resolve "$D:443:127.0.0.1" --resolve "$D:80:127.0.0.1" -L "http://$D/a125-mail.php" | head -c 300; echo
rm -f "$DR/a125-mail.php"
echo "--- 1b. PHP mail() as $SU with php-cli"
if command -v php >/dev/null; then runuser -u $SU -- php -r 'var_dump(mail("'"$EXT"'", "a125 php-cli mail", "body", "From: wordpress@'"$D"'"));' 2>&1 | sed 's/^/  /'; else echo "  (no php-cli on this server)"; fi
echo "--- 1c. sendmail as $SU"
printf 'Subject: a125 site sendmail\n\nsite\n' | runuser -u $SU -- /usr/sbin/sendmail -i "$EXT" 2>&1 | sed 's/^/  /'; echo "  exit ${PIPESTATUS[1]}"
smtp() { python3 - "$@" <<'P'
import smtplib, sys
frm, to, subj = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    s = smtplib.SMTP("127.0.0.1", 25, timeout=30); s.ehlo("localhost")
    r = s.sendmail(frm, [to], "From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nno password\r\n" % (frm, to, subj))
    print("  ", r or "250 accepted"); s.quit()
except smtplib.SMTPRecipientsRefused as e:
    for k, v in e.recipients.items(): print("   refused", k, v[0], v[1].decode(errors="replace"))
except Exception as e: print("   error", type(e).__name__, e)
P
}
echo "--- 2a. SMTP 127.0.0.1:25 without a password into this server's own mailbox $LOCAL"; smtp contact@$D $LOCAL "a125 local delivery"
echo "--- 2b. SMTP 127.0.0.1:25 without a password to another server ($EXT): relay from mynetworks"; smtp contact@$D $EXT "a125 relay no auth"
echo "--- 3a. root sendmail (system notice)"; printf 'Subject: a125 root notice\n\nroot\n' | /usr/sbin/sendmail -i "$EXT"; echo "  exit $?"
echo "--- 3b. vmail sendmail (Sieve vacation path)"; printf 'Subject: a125 vmail reply\n\nvmail\n' | runuser -u vmail -- /usr/sbin/sendmail -i "$EXT"; echo "  exit $?"
sleep 8; postqueue -f; sleep 8
echo "--- mail log (refusals and deliveries, last 3 min)"
journalctl -q -o short-iso --since -3min --no-pager | grep -E "not allowed to submit|Relay access denied|status=(sent|deferred|bounced)|uid=[0-9]+ from=" | cut -c1-250 | tail -20
echo "--- queue: $(postqueue -j | wc -l) message(s)"
echo "--- $LOCAL maildir files: $(find /var/mail /var/vmail /home/vmail -xdev -type f -path "*${LOCAL#*@}*${LOCAL%@*}*" 2>/dev/null | grep -v dovecot | wc -l)"
