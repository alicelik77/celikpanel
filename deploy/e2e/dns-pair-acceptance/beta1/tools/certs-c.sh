for pair in "192.0.3.30 frankfurt.beta1c.test" "192.0.3.31 boston.beta1c.test"; do set -- $pair
  echo "== $2:"; echo | openssl s_client -connect $1:2083 -servername $2 -CAfile /root/beta1/pebble-root.pem 2>&1 | grep -E 'Verify return code|issuer=' | head -2
done
