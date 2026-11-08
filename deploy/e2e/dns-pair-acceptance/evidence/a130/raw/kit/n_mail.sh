# labnet (root): mail checks against A. usage: n_mail.sh autoconf DOMAIN IP | dig NAME TYPE IP | swaks FROM TO [SERVER] [PORT] | relaylog N. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
case "$1" in
autoconf) D=$2; IP=$3
  echo "-- http://autoconfig.$D/mail/config-v1.1.xml?emailaddress=x@$D"
  curl -sS -m 10 --resolve autoconfig.$D:80:$IP "http://autoconfig.$D/mail/config-v1.1.xml?emailaddress=info@$D" -w '\n[http %{http_code}]\n' | grep -E "hostname|port|socketType|username|http |password" | head -12
  echo "-- https://$D/.well-known/autoconfig/mail/config-v1.1.xml"
  curl -sSk -m 10 --resolve $D:443:$IP "https://$D/.well-known/autoconfig/mail/config-v1.1.xml" -w '\n[http %{http_code}]\n' | grep -E "hostname|http " | head -4
  echo "-- http://$D/.well-known/autoconfig/mail/config-v1.1.xml"
  curl -sS -m 10 --resolve $D:80:$IP "http://$D/.well-known/autoconfig/mail/config-v1.1.xml" -o /dev/null -w '[http %{http_code} %{redirect_url}]\n'
  echo "-- Autodiscover POST (https autodiscover.$D, -k)"
  B="<?xml version=\"1.0\" encoding=\"utf-8\"?><Autodiscover xmlns=\"http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006\"><Request><EMailAddress>secret-probe@$D</EMailAddress><AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema></Request></Autodiscover>"
  out=$(curl -sSk -m 10 --resolve autodiscover.$D:443:$IP -X POST -H 'Content-Type: text/xml' --data "$B" "https://autodiscover.$D/autodiscover/autodiscover.xml" -w '\n[http %{http_code}]')
  echo "$out" | tail -1; echo "$out" | grep -oE "<(Server|Port|SSL|Type|Encryption)>[^<]*" | head -8 | xargs
  echo "LoginName present: $(echo "$out" | grep -c LoginName) ; asked address echoed: $(echo "$out" | grep -c secret-probe)"
  out=$(curl -sS -m 10 --resolve autodiscover.$D:80:$IP -X POST -H 'Content-Type: text/xml' --data "$B" "http://autodiscover.$D/autodiscover/autodiscover.xml" -w '\n[http %{http_code}]'); echo "http POST: $(echo "$out" | tail -1)" ;;
dig) dig +short +time=2 +tries=1 +norecurse @$4 $2 $3 | head -4 | xargs ;;
swaks) F=$2; TO=$3; S=${4:-192.0.3.190}; P=${5:-25}
  swaks --server $S --port $P --from "$F" --to "$TO" --h-Subject "a130 lab $(date +%s%N)" --body "a130 lab message" --timeout 30 2>&1 | grep -E "^<[-~*]* *(250 2\.0\.0|5[0-9][0-9]|4[0-9][0-9])|\*\*\*" | grep -vE "^<[-~]* *250-" | tail -2 | cut -c1-200 ;;
relaylog) tail -n ${2:-6} /var/log/lab-relay.log 2>/dev/null | grep -E "sasl_username|status=|warning|error|reject" | cut -c1-260 ;;
esac
