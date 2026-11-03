# on labnet: the certificate served on 25 (STARTTLS), 587 (STARTTLS) and 993 for the mail host name. Lab only.
IP=$1; N=$2
for spec in "25 -starttls smtp" "587 -starttls smtp" "993"; do
  set -- $spec; p=$1; shift
  echo "## port $p $*"
  echo Q | timeout 15 openssl s_client -connect $IP:$p -servername $N -CAfile /root/beta1/pebble-root.pem -verify_hostname $N "$@" 2>/dev/null > /tmp/tls.$p
  openssl x509 -in /tmp/tls.$p -noout -subject -issuer -ext subjectAltName 2>/dev/null | tr '\n' ' '; echo
  grep -E 'Verify return code|Verification' /tmp/tls.$p | head -2
done
ls /root/beta1/pebble-root.pem
