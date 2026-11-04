# guest (root, read only): modes of a site's document root tree. usage: g_mode6.sh DOMAIN. Lab only.
f=$(grep -l "server_name.* $1" /etc/nginx/sites-enabled/* 2>/dev/null | head -1); r=$(grep -m1 ' root ' "$f" | awk '{print $2}' | tr -d ';')
echo "## $(date -u +%T) $1 root $r"; stat -c '%n %U:%G %A %a' "$r"
echo "dirs by mode: $(find "$r" -type d -printf '%m\n' | sort | uniq -c | tr '\n' ' ')"
echo "dirs not 2750: $(find "$r" -type d ! -perm 2750 | head -5 | tr '\n' ' ')"
echo "files with setuid/setgid: $(find "$r" -type f \( -perm -4000 -o -perm -2000 \) | wc -l)"
echo "files by mode: $(find "$r" -type f -printf '%m\n' | sort | uniq -c | tr '\n' ' ')"
echo "foreign owner/group: $(find "$r" \( ! -user "$(stat -c %U "$r")" -o ! -group www-data \) | head -5 | tr '\n' ' ')"
