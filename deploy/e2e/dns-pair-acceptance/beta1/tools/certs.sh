# on labnet: what each panel serves by IP and by name
for pair in "192.0.3.10 frankfurt.beta1a.test" "192.0.3.11 boston.beta1a.test"; do set -- $pair
  echo "== $1 no SNI:"; echo | openssl s_client -connect $1:${PANEL_PORT:-1923} -noservername 2>/dev/null | openssl x509 -noout -subject -issuer -fingerprint -sha256 -ext subjectAltName 2>/dev/null
  echo "== $1 SNI $2:"; echo | openssl s_client -connect $1:${PANEL_PORT:-1923} -servername $2 2>/dev/null | openssl x509 -noout -subject -issuer -dates -ext subjectAltName 2>/dev/null
  echo "== verify $2 against pebble root:"; echo | openssl s_client -connect $1:${PANEL_PORT:-1923} -servername $2 -CAfile /root/beta1/pebble-root.pem -verify_return_error 2>&1 | grep -E 'Verify return code|verify error' | head -2
done
journalctl -u beta1-pebble --no-pager -o short-iso | grep -iE 'issued|Issuing|certificate' | tail -8
