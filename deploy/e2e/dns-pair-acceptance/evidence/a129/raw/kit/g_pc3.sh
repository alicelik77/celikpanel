D=$1
c() { curl -sk -D - -o /tmp/pc.out --resolve $D:80:127.0.0.1 --resolve $D:443:127.0.0.1 -L "http://$D$1" | grep -iE "^HTTP|x-cache" | tr -d '\r' | tr '\n' ' '; head -c 40 /tmp/pc.out; echo; }
echo "clock 1: $(c /clock.php)"; sleep 1; echo "clock 2: $(c /clock.php)"; echo "clock with a session cookie: $(curl -sk -D - -o /dev/null -H 'Cookie: PHPSESSID=x' --resolve $D:80:127.0.0.1 -L http://$D/clock.php | grep -i x-cache | tr -d '\r')"
find /var/cache/celikpanel/pagecache -type f | wc -l
