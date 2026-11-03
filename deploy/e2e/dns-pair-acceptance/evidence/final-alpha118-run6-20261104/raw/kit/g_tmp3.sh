grep -h "wp118" /var/log/nginx/error.log 2>/dev/null | tail -5 | cut -c1-300
ls /run/php/ 2>/dev/null; systemctl is-active php8.4-fpm
