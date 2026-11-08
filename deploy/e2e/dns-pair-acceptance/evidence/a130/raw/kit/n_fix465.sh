sed -i 's/smtpd_tls_wrapper_mode=yes/smtpd_tls_wrappermode=yes/' /etc/postfix/master.cf; systemctl restart postfix; sleep 2
echo | timeout 8 openssl s_client -connect 192.0.3.1:465 -quiet 2>&1 | head -2
