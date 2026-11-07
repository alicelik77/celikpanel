# guest (root, read-only): site7.test's PHP wiring. Lab only.
grep -rn "fastcgi_pass" /etc/nginx/sites-enabled/ /etc/nginx/celikpanel-sites.d/ 2>/dev/null | grep -i site7 | head
grep -rln "site7" /etc/nginx/ 2>/dev/null | head
ls -la /etc/php/8.3/fpm/pool.d/ /etc/php/8.4/fpm/pool.d/ 2>&1 | grep -E "site4|pool.d"
ls -la /var/run/php/ | grep site4
systemctl is-active php8.3-fpm php8.4-fpm
curl -sS -m 5 -o /dev/null -w "local http %{http_code}\n" -H "Host: site7.test" http://127.0.0.1/
curl -sSk -m 5 -o /dev/null -w "local https %{http_code}\n" -H "Host: site7.test" https://127.0.0.1/
tail -3 /var/log/nginx/error.log | cut -c1-300
