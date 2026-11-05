# guest (root): case 7 sending paths of the site user, to another server. $1 domain, $2 site user, $3 recipient. Lab only.
D=$1; U=$2; TO=$3
echo "--- 1. PHP mail() as the site user $U (php-cli; FPM uses the same sendmail_path)"
runuser -u "$U" -- php -r 'var_dump(mail($argv[1], "a124 php mail()", "sent by the site with PHP mail()", "From: wordpress@" . $argv[2]));' "$TO" "$D" 2>&1 | sed 's/^/  /'
echo "--- 2. SMTP to 127.0.0.1:25 without a password, to another server"
python3 -c '
import smtplib, sys
d, to = sys.argv[1], sys.argv[2]
s = smtplib.SMTP("127.0.0.1", 25, timeout=30); s.ehlo("localhost")
msg = "From: contact@%s\r\nTo: %s\r\nSubject: a124 loopback 25\r\n\r\nno password\r\n" % (d, to)
print(" ", s.sendmail("contact@" + d, [to], msg) or "250 accepted"); s.quit()' "$D" "$TO"
sleep 8; postqueue -f; sleep 8
echo "queue: $(postqueue -j | wc -l)"
echo "postconf: $(postconf -h authorized_submit_users smtpd_relay_restrictions | tr '\n' ';')"
journalctl -q -o short-iso --since -2min --no-pager | grep -E "postfix/(pickup|smtpd|qmgr|smtp)\[" | grep -E "uid=|client=localhost|from=<(wordpress|contact)@|relay=" | tail -10 | cut -c1-260
