# guest (root): case 7 sending paths for one site. $1 domain, $2 docroot, $3 recipient. Lab only (sends mail as a site would).
D=$1; DR=$2; TO=$3
OWN=$(stat -c %U "$DR"); echo "docroot $DR owner $OWN"
printf '<?php var_dump(mail("%s", "a124 php mail()", "sent by the site with PHP mail()", "From: wordpress@%s"));\n' "$TO" "$D" > "$DR/a124-mail.php"
chown $OWN: "$DR/a124-mail.php"
echo "--- 1. PHP mail() from the site (FPM as $OWN)"
curl -s -k -m 30 --resolve "$D:443:127.0.0.1" "https://$D/a124-mail.php" | head -c 300; echo
rm -f "$DR/a124-mail.php"
echo "--- 2. SMTP to 127.0.0.1:25 without a password"
python3 -c '
import smtplib, sys
d, to = sys.argv[1], sys.argv[2]
s = smtplib.SMTP("127.0.0.1", 25, timeout=30); s.ehlo("localhost")
msg = "From: contact@%s\r\nTo: %s\r\nSubject: a124 loopback 25\r\n\r\nno password\r\n" % (d, to)
print(" ", s.sendmail("contact@" + d, [to], msg) or "250 accepted"); s.quit()' "$D" "$TO"
sleep 5; postqueue -f; sleep 5
echo "queue: $(postqueue -j | wc -l)"
echo "postconf: $(postconf -h authorized_submit_users smtpd_relay_restrictions mynetworks | tr '\n' ';')"
journalctl -q -o short-iso --since -3min --no-pager | grep -E "postfix/(pickup|smtpd|qmgr|lmtp|local|smtp|cleanup)" | grep -E "uid=|client=localhost|from=<(wordpress|contact)@|status=" | tail -10 | cut -c1-240
