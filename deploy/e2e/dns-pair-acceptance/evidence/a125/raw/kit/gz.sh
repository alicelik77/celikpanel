# case 3 compression check from the lab host (curl, the browser's Accept-Encoding: gzip). Lab only; prints headers only, never bodies.
# usage: gz.sh CELL NODE
S=/var/tmp/cp-beta1/state/$1/$2.json
read IP PORT CK < <(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["ip"], d.get("panel_port") or 1923, d["cookie"])' $S)
Bu=https://$IP:$PORT
hdr() { # METHOD PATH [BODY]
  local m=$1 p=$2 b=${3:-}
  if [ "$m" = POST ]; then
    curl -sk -m 120 -o /tmp/gzbody -D /tmp/gzh -X POST -H "Accept-Encoding: gzip, deflate, br" -H "Cookie: celikpanel_session=$CK" -H "Origin: $Bu" -H "Content-Type: application/json" -H "X-CelikPanel-Request-Id: $(openssl rand -hex 16)" --data "$b" "$Bu$p"
  else
    curl -sk -m 120 -o /tmp/gzbody -D /tmp/gzh -H "Accept-Encoding: gzip, deflate, br" -H "Cookie: celikpanel_session=$CK" "$Bu$p"
  fi
  printf '  %-6s %-58s %s | %s | %s | wire %s B\n' "$m" "$p" "$(head -1 /tmp/gzh | tr -d '\r')" "$(grep -i '^content-encoding:' /tmp/gzh | tr -d '\r' || echo 'content-encoding: -')" "$(grep -i '^content-type:' /tmp/gzh | tr -d '\r' | cut -c1-45)" "$(stat -c %s /tmp/gzbody)"
}
echo "== $1/$2 $Bu"
hdr GET /
JS=$(curl -sk -H "Cookie: celikpanel_session=$CK" $Bu/ | grep -oE '/assets/[^"]+\.js' | head -1); CSS=$(curl -sk $Bu/ | grep -oE '/assets/[^"]+\.css' | head -1)
hdr GET $JS; hdr GET $CSS
hdr GET /api/v1/managed-services
hdr POST /api/v1/managed-services/scan '{}'
hdr GET /api/v1/hosting/capabilities
hdr GET /api/v1/setup/components
hdr GET /api/v1/domains
echo "-- secret-bearing answers"
DBS=$(curl -sk -H "Cookie: celikpanel_session=$CK" $Bu/api/v1/database-servers | python3 -c 'import json,sys
try:
  d=json.load(sys.stdin); L=d if isinstance(d,list) else (d.get("servers") or d.get("database_servers") or [])
  print(" ".join(str(s.get("id")) for s in L if isinstance(s,dict)))
except Exception: print("")')
for id in $DBS; do hdr GET /api/v1/database-servers/$id/admin-account; done
hdr GET /api/v1/auth/me
rm -f /tmp/gzbody /tmp/gzh
