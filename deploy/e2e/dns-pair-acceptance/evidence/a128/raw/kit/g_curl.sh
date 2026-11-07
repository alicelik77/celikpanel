# guest: one request to a site on this server. usage: g_curl.sh DOMAIN PATH [grep-header]
D=$1; P=$2
curl -sk -D /tmp/h.out -o /tmp/b.out -w "$P -> %{http_code}\n" --resolve $D:80:127.0.0.1 --resolve $D:443:127.0.0.1 -L "http://$D$P"
[ -n "${3:-}" ] && grep -i "$3" /tmp/h.out | tr -d '\r'
head -c 100 /tmp/b.out | tr '\n' ' '; echo
