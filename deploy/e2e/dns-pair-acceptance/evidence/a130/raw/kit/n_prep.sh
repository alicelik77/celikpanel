# labnet (root): database clients + a local SMTP relay with SASL on 192.0.3.1:587/465 for the smarthost check.
# The relay accepts authenticated mail and discards it (logged). usage: n_prep.sh RELAYUSER (password on stdin). Lab only.
export DEBIAN_FRONTEND=noninteractive PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
U=$1; IFS= read -r PW
echo "postfix postfix/main_mailer_type select No configuration" | debconf-set-selections
apt-get install -y -q postgresql-client mariadb-client postfix sasl2-bin libsasl2-modules ssl-cert >/dev/null 2>&1; echo "apt rc $?"
command -v psql mariadb postconf saslpasswd2 | xargs
[ -f /etc/postfix/main.cf ] || cp /usr/share/postfix/main.cf.debian /etc/postfix/main.cf
postconf -e "myhostname = relay.lab.test" "mydestination =" "inet_interfaces = 192.0.3.1" "inet_protocols = ipv4" \
  "smtpd_tls_cert_file = /etc/ssl/certs/ssl-cert-snakeoil.pem" "smtpd_tls_key_file = /etc/ssl/private/ssl-cert-snakeoil.key" \
  "smtpd_sasl_auth_enable = yes" "smtpd_sasl_type = cyrus" "smtpd_sasl_path = smtpd" "smtpd_tls_auth_only = yes" \
  "smtpd_relay_restrictions = permit_sasl_authenticated, reject" "smtpd_recipient_restrictions = permit_sasl_authenticated, reject" \
  "default_transport = discard:lab relay sink" "relay_transport = discard:lab relay sink" "maillog_file = /var/log/lab-relay.log"
printf 'pwcheck_method: auxprop\nauxprop_plugin: sasldb\nmech_list: PLAIN LOGIN\n' > /etc/postfix/sasl/smtpd.conf
echo "$PW" | saslpasswd2 -p -c -u relay.lab.test "$U"; chgrp postfix /etc/sasldb2; chmod 640 /etc/sasldb2
sed -i -E 's/^smtp( +)inet(.*)$/#smtp\1inet\2/' /etc/postfix/master.cf
grep -q '^submission inet' /etc/postfix/master.cf || cat >> /etc/postfix/master.cf <<'M'
submission inet n       -       n       -       -       smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
smtps     inet  n       -       n       -       -       smtpd
  -o syslog_name=postfix/smtps
  -o smtpd_tls_wrappermode=yes
M
systemctl restart postfix; sleep 2; ss -ltn | grep -E ':(587|465|25) ' | awk '{print $4}' | xargs
echo "relay user: $U@relay.lab.test"
