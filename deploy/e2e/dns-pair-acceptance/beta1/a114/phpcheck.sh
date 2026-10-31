#!/bin/bash
# on labnet: a PHP page of the site over https, and http -> https. Lab only.
S=$1; CA=/root/beta1/pebble-root.pem
IP=$(dig +short @192.0.2.53 $S A | grep -E '^[0-9.]+$' | tail -1)
R="--resolve $S:80:$IP --resolve $S:443:$IP"
echo "--- https://$S/hello.php"
curl -sS $R --max-time 20 --cacert $CA -D /tmp/pc.hdr -o /tmp/pc.body -w 'code=%{http_code}\n' https://$S/hello.php
grep -iE '^(HTTP|server|x-powered-by|content-type)' /tmp/pc.hdr | tr -d '\r'
echo "x-powered-by present: $(grep -ic '^x-powered-by' /tmp/pc.hdr)"
echo "body: $(head -c 200 /tmp/pc.body)"
echo "--- http://$S/hello.php (redirect?)"
curl -sS $R --max-time 20 -o /dev/null -D - -w 'code=%{http_code} location=%{redirect_url}\n' http://$S/hello.php | grep -iE '^(HTTP|location|server)|code=' | tr -d '\r'
