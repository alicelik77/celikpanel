# guest (root): case 7 authenticated submission 587 with the sending mailbox (password on stdin). Lab only.
U=$1; TO=$2
PW=$(cat)
printf '%s' "$PW" | python3 -c '
import smtplib, ssl, sys
u, to = sys.argv[1], sys.argv[2]; pw = sys.stdin.read().strip()
c = ssl.create_default_context(); c.check_hostname = False; c.verify_mode = ssl.CERT_NONE
s = smtplib.SMTP("127.0.0.1", 587, timeout=30); s.ehlo("lab"); print(" starttls", s.starttls(context=c)[0]); s.ehlo("lab")
print(" auth", s.login(u, pw)[0])
msg = "From: %s\r\nTo: %s\r\nSubject: a124 587 authenticated\r\n\r\nwith password\r\n" % (u, to)
print(" send", s.sendmail(u, [to], msg) or "250 accepted"); s.quit()' "$U" "$TO"
sleep 5; journalctl -q -o short-iso --since -2min --no-pager | grep -E "sasl_username" | tail -2 | cut -c1-240
