#!/bin/bash
# a125 lab, guest side: a stock Debian 13 mail server shaped like CelikPanel's
# (Postfix + Dovecot SASL submission, vmail virtual mailboxes), official Debian
# packages only. An "outside" domain is routed to a local SMTP sink so relayed
# mail can be seen without the Internet. Lab only.
set -eu
export DEBIAN_FRONTEND=noninteractive
rm -f /etc/apt/sources.list.d/php.list   # the base image's third-party PHP repo: not used
apt-get update -q >/dev/null
echo "postfix postfix/main_mailer_type select Internet Site" | debconf-set-selections
echo "postfix postfix/mailname string mx.lab.test" | debconf-set-selections
apt-get install -y -q postfix dovecot-core php-cli swaks >/dev/null
hostnamectl set-hostname mx.lab.test || true
groupadd -g 5000 vmail 2>/dev/null || true
useradd -r -g 5000 -u 5000 -d /var/mail/vhosts -s /usr/sbin/nologin vmail 2>/dev/null || true
mkdir -p /var/mail/vhosts && chown vmail:vmail /var/mail/vhosts
useradd -m -s /bin/bash shop_test 2>/dev/null || true   # the site user of shop.test (services.SiteUsername)
printf 'owner@shop.test shop.test/owner/\nnoreply@shop.test shop.test/noreply/\n' > /etc/postfix/vmailbox
printf 'outside.test smtp:[127.0.0.1]:2526\n' > /etc/postfix/transport
postconf -e myhostname=mx.lab.test mydestination=localhost virtual_mailbox_domains=shop.test \
  virtual_mailbox_maps=texthash:/etc/postfix/vmailbox virtual_mailbox_base=/var/mail/vhosts \
  virtual_uid_maps=static:5000 virtual_gid_maps=static:5000 virtual_minimum_uid=100 \
  transport_maps=texthash:/etc/postfix/transport smtp_tls_security_level=none
postconf -M submission/inet="submission inet n - y - - smtpd"
postconf -P submission/inet/syslog_name=postfix/submission submission/inet/smtpd_tls_security_level=encrypt \
  submission/inet/smtpd_sasl_auth_enable=yes submission/inet/smtpd_sasl_type=dovecot \
  submission/inet/smtpd_sasl_path=private/auth "submission/inet/smtpd_client_restrictions=permit_sasl_authenticated,reject" \
  "submission/inet/smtpd_relay_restrictions=permit_sasl_authenticated,reject"
cat > /etc/dovecot/conf.d/99-lab.conf <<'EOF'
auth_mechanisms = plain login
auth_allow_cleartext = yes
passdb passwd-file {
  passwd_file_path = /etc/dovecot/users
}
userdb passwd-file {
  passwd_file_path = /etc/dovecot/users
}
service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0660
    user = postfix
    group = postfix
  }
}
EOF
printf 'noreply@shop.test:{PLAIN}lab-pass-not-secret:5000:5000::/var/mail/vhosts/shop.test/noreply\n' > /etc/dovecot/users
chmod 640 /etc/dovecot/users; chown root:dovecot /etc/dovecot/users
# The outside world: a tiny SMTP sink that keeps every message as a file.
mkdir -p /root/sink
cat > /root/sink.py <<'EOF'
import socket, threading, time, os
def serve(c):
    f = c.makefile('rwb'); w = lambda s: (f.write(s.encode() + b'\r\n'), f.flush())
    w('220 sink.outside.test ESMTP'); data = None
    while True:
        line = f.readline()
        if not line: break
        cmd = line.decode(errors='replace').strip()
        if data is not None:
            if cmd == '.':
                open('/root/sink/%d.eml' % time.time_ns(), 'w').write(''.join(data)); data = None; w('250 kept')
            else: data.append(line.decode(errors='replace'))
            continue
        u = cmd.upper()
        if u.startswith('EHLO') or u.startswith('HELO'): w('250 sink.outside.test')
        elif u == 'DATA': data = []; w('354 go')
        elif u == 'QUIT': w('221 bye'); break
        else: w('250 ok')
    c.close()
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.bind(('127.0.0.1', 2526)); s.listen(8)
while True:
    c, _ = s.accept(); threading.Thread(target=serve, args=(c,), daemon=True).start()
EOF
systemd-run --unit=lab-sink python3 /root/sink.py >/dev/null
systemctl restart dovecot postfix
sleep 2
echo "setup done: postfix $(postconf -h mail_version), dovecot $(dovecot --version | cut -d' ' -f1), php $(php -r 'echo PHP_VERSION;')"
echo "php sendmail_path: $(php -r 'echo ini_get("sendmail_path");')"
