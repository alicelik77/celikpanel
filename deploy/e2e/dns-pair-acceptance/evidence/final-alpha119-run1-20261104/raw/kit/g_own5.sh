# guest (root, read only): document root files with owner, group and mode. Lab only.
f=$(grep -l "server_name.* $1" /etc/nginx/sites-enabled/* 2>/dev/null | head -1); r=$(grep -m1 ' root ' "$f" | awk '{print $2}' | tr -d ';')
echo "## $(date -u +%T) $1 root $r"; stat -c '%n %U:%G %a' "$r" "$r"/* 2>&1 | head -12
for x in restore-probe.txt; do [ -f "$r/$x" ] && echo "$x content: $(cat $r/$x)"; done
find "$r" -maxdepth 3 ! -user "$(stat -c %U "$r")" 2>/dev/null | head -5 | sed 's/^/foreign owner: /'
