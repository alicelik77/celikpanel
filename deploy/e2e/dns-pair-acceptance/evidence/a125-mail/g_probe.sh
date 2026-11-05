#!/bin/bash
# a125 lab, guest side: the five sending paths, one line each. $1 = label.
set -u
L=$1
sinkcount() { ls /root/sink | wc -l; }
waitq() { for i in $(seq 1 30); do [ -z "$(postqueue -j)" ] && return 0; postqueue -f; sleep 1; done; echo "  (queue not empty: $(postqueue -j | wc -l))"; }
echo "=== $L $(date -u +%FT%TZ)"
echo "postconf: $(postconf authorized_submit_users smtpd_relay_restrictions | tr '\n' ';')"
n0=$(sinkcount)
echo "--- 1. site PHP mail() as shop_test (php-cli; FPM uses the same sendmail_path)"
runuser -u shop_test -- php -r 'var_dump(mail("site@outside.test", "'"$L"' php mail", "body", "From: wordpress@shop.test"));' 2>&1 | sed 's/^/  /'
echo "--- 2. site SMTP to 127.0.0.1:25 without a password, to another server"
swaks --server 127.0.0.1:25 --from contact@shop.test --to relay@outside.test --header "Subject: $L smtp25" 2>&1 | grep -E '^ *(<-|<~|<\*\*) +[245][0-9][0-9]' | tail -3 | sed 's/^/  /'
echo "--- 2b. SMTP to 127.0.0.1:25 without a password, into this server's own mailbox"
swaks --server 127.0.0.1:25 --from contact@shop.test --to owner@shop.test --header "Subject: $L local" 2>&1 | grep -E '^ *(<-|<~|<\*\*) +[245][0-9][0-9]' | tail -2 | sed 's/^/  /'
echo "--- 3. authenticated submission 587 (STARTTLS) with the mailbox password"
swaks --server 127.0.0.1:587 -tls --auth PLAIN --auth-user noreply@shop.test --auth-password lab-pass-not-secret \
  --from noreply@shop.test --to auth@outside.test --header "Subject: $L 587" 2>&1 | grep -E '^ *(<-|<~|<\*\*) +(235|250 .*queued|[45][0-9][0-9])' | tail -2 | sed 's/^/  /'
echo "--- 4. system notice as root (cron, unattended-upgrades, fail2ban use this path)"
printf 'Subject: %s root notice\n\nfrom root\n' "$L" | /usr/sbin/sendmail -i root-notice@outside.test; echo "  exit $?"
echo "--- 5. mailbox owner vmail (Dovecot Sieve out-of-office replies use this path)"
printf 'Subject: %s vmail reply\n\nfrom vmail\n' "$L" | runuser -u vmail -- /usr/sbin/sendmail -i vacation@outside.test; echo "  exit $?"
waitq
n1=$(sinkcount)
echo "--- outside sink received $((n1-n0)) new message(s):"
for f in $(ls -t /root/sink | head -$((n1-n0))); do grep -m1 '^Subject:' /root/sink/$f | sed 's/^/  /'; done
echo "--- owner@shop.test maildir: $(find /var/mail/vhosts/shop.test/owner -type f 2>/dev/null | wc -l) message(s)"
