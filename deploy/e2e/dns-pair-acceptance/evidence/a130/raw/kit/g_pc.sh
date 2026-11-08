D=$1
cat /etc/nginx/celikpanel-managed.d/$D/settings.site
echo "--- http-level cache config"; grep -rn "fastcgi_cache_path\|keys_zone\|celikpanel.*cache" /etc/nginx/nginx.conf /etc/nginx/conf.d/ 2>/dev/null | head
ls -la /var/cache/nginx/ 2>&1 | head; ls -la /var/cache/celikpanel* 2>&1 | head
curl -sk -o /dev/null -w "clock %{http_code}\n" --resolve $D:80:127.0.0.1 --resolve $D:443:127.0.0.1 -L http://$D/clock.php
echo "--- error log"; tail -5 /var/log/nginx/error.log | cut -c1-400; ls /var/log/nginx/ | head; tail -3 /var/log/nginx/*$D*error* 2>/dev/null | cut -c1-400
