# guest (root, read-only): why wp.r9.test answers 500 after the database password change. Lab only. Prints no secret.
R=$(ls -d /var/www/celikpanel/subscriptions/*/sites/4/public_html); echo "root $R"
grep -E "define\( *'DB_(NAME|USER|HOST|CHARSET)'" $R/wp-config.php
grep -E "DB_PASSWORD" $R/wp-config.php | sed -E "s/(DB_PASSWORD'?\"?, *)(['\"]).*(['\"])/\1\2<len:$(grep DB_PASSWORD $R/wp-config.php | head -1 | wc -c)>\3/"
grep -E "DB_PASSWORD" $R/wp-config.php | head -1 | grep -o "[\\\$'\"]" | sort | uniq -c | tr '\n' ' '; echo
php -r '$s=file_get_contents($argv[1]); preg_match_all("/define\(\s*[\x27\"](DB_\w+)[\x27\"]\s*,\s*(.+?)\s*\);/",$s,$m,PREG_SET_ORDER); foreach($m as $x){ eval("define(\"".$x[1]."\",".$x[2].");"); } $c=@mysqli_connect(DB_HOST,DB_USER,DB_PASSWORD,DB_NAME); echo $c ? "php connect with wp-config: ok\n" : "php connect with wp-config: ".mysqli_connect_error()."\n";' $R/wp-config.php 2>&1 | head -3
ls /var/log/nginx/ 2>/dev/null | head; tail -n 5 /var/log/nginx/error.log 2>/dev/null | cut -c1-300
find /var/www/celikpanel/subscriptions/*/sites/4 -maxdepth 2 -name "*.log" 2>/dev/null | head -3
journalctl --since "20:03:50" -o cat --no-pager | grep -iE "php|fpm|wp\.r9" | tail -6 | cut -c1-300
