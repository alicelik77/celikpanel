#!/bin/bash
# on labnet: what a visitor sees for a site (lab resolver, pebble root as trust). Lab only.
S=$1
CA=/root/beta1/pebble-root.pem
IP=$(dig +short @192.0.2.53 $S A | grep -E '^[0-9.]+$' | tail -1)
WIP=$(dig +short @192.0.2.53 www.$S A | grep -E '^[0-9.]+$' | tail -1)
echo "resolves (lab resolver 192.0.2.53): $S=$IP www.$S=${WIP:-none}"
R="--resolve $S:80:$IP --resolve $S:443:$IP"
[ -n "$WIP" ] && R="$R --resolve www.$S:80:$WIP --resolve www.$S:443:$WIP"
for u in http://$S/ https://$S/ http://www.$S/ https://www.$S/; do
  echo "--- $u"
  rm -f /tmp/wc.body /tmp/wc.hdr
  curl -sS $R --max-time 20 --cacert $CA -o /tmp/wc.body -D /tmp/wc.hdr -w 'code=%{http_code} redirect=%{redirect_url} ssl_verify=%{ssl_verify_result}\n' "$u" 2>&1
  grep -iE '^(HTTP|server|x-powered-by|location|strict-transport|content-type)' /tmp/wc.hdr 2>/dev/null | tr -d '\r'
  if grep -q '^HTTP.* 200' /tmp/wc.hdr 2>/dev/null; then
    echo "body: $(wc -c < /tmp/wc.body) bytes, title: $(grep -oiE '<title>[^<]*' /tmp/wc.body | head -1)"
    echo "software words in body: [$(grep -oiE 'php[^ <]*|nginx[^ <]*|apache|celikpanel|powered by|version [0-9.]+|debian|ubuntu|arch linux' /tmp/wc.body | sort -u | tr '\n' ' ')]"
  fi
done
echo "--- certificate served on $S:443"
echo | openssl s_client -connect $IP:443 -servername $S -CAfile $CA 2>/dev/null | openssl x509 -noout -subject -ext subjectAltName -issuer -enddate 2>&1 | tr '\n' ' '; echo
echo | openssl s_client -connect $IP:443 -servername $S -CAfile $CA 2>&1 | grep -E 'Verify return code' | head -1
