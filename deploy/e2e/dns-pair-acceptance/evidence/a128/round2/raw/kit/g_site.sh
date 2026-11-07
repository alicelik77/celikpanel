# guest (root, read-only): the docroot and system user of a site from nginx. $1 domain
f=$(grep -lE "server_name[^;]*[[:space:]]$1[[:space:];]" /etc/nginx/sites-enabled/* /etc/nginx/conf.d/*.conf 2>/dev/null | head -1)
r=$(grep -m1 -E '^\s*root\s' "$f" | awk '{print $2}' | tr -d ';')
echo "$r $(stat -c %U "$r" 2>/dev/null)"
