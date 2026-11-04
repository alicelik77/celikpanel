# guest (root, read only): document root listing of a domain and databases count. Lab only.
f=$(grep -l "server_name.* $1" /etc/nginx/sites-enabled/* 2>/dev/null | head -1); r=$(grep -m1 ' root ' "$f" | awk '{print $2}' | tr -d ';'); echo "## $1 root $r"; ls -la "$r" 2>&1 | head -12
mysql -N -e "show databases" 2>/dev/null | grep -vcE "^(information_schema|mysql|performance_schema|sys)$" | sed 's/^/databases (non-system): /'
